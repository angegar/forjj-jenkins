package main

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/forj-oss/forjj-modules/trace"
	"gopkg.in/yaml.v2"
)

const template_file = "templates.yaml"

// Contains functions to manage source code from templates

type YamlTemplates struct {
	Defaults DefaultsStruct
	Features TmplFeatures
	Sources  TmplSources
	Run      map[string]RunStruct `yaml:"run_deploy"`
	Variants map[string]string
}

type DefaultsStruct struct {
	Dockerfile   DockerfileStruct
	JenkinsImage FinalImageStruct
}

type TmplFeatures struct {
	Common TmplFeaturesStruct
	Deploy map[string]TmplFeaturesStruct
}

type TmplFeaturesStruct []string

type TmplSources struct {
	Common TmplSourcesStruct
	Deploy map[string]TmplSourcesStruct
}

type TmplSourcesStruct map[string]TmplSource

type TmplSource struct {
	Chmod    os.FileMode
	Template string
	Tag      string // template string to use.
	Source   string
	If       string `yaml:"if"` // If `If` is empty, the file will be ignored. otherwise the file will copied/generated
	// as usual.
}

type EnvStruct struct {
	Value string
	If    string
}

type RunStruct struct {
	RunCommand string               `yaml:"run"`
	Env        map[string]EnvStruct `yaml:"environment"`
	Files      RunFilesStruct
}

// Model creates the Model data used by gotemplates in maintain context.
func (p *JenkinsPlugin) Model() (model *JenkinsPluginModel) {
	if JP_Model != nil {
		return JP_Model
	}
	JP_Model = new(JenkinsPluginModel)
	JP_Model.Creds = make(map[string]string)
	JP_Model.Env = make(map[string]string)
	JP_Model.Config = p.yaml
	return JP_Model
}

// SourceModel creates the Model data used by gotemplates in create/update context.
// The model is not updated until call to CleanModel()
func (p *JenkinsPlugin) SourceModel() *JenkinsPluginSourceModel {
	if JPS_Model != nil {
		return JPS_Model
	}
	JPS_Model = new(JenkinsPluginSourceModel)
	JPS_Model.Source = p.yaml
	return JPS_Model
}

func (p *JenkinsPlugin) CleanSourceModel() {
	JPS_Model = nil
}

func Evaluate(value string, data interface{}) (string, error) {
	var doc bytes.Buffer
	tmpl := template.New("jenkins_plugin_data")

	if !strings.Contains(value, "{{") {
		return value, nil
	}
	if _, err := tmpl.Parse(value); err != nil {
		return "", err
	}
	if err := tmpl.Execute(&doc, data); err != nil {
		return "", err
	}
	ret := doc.String()
	log.Printf("'%s' were interpreted to '%s'", value, ret)
	return ret, nil
}

//Load templates definition file from template dir.
func (p *JenkinsPlugin) LoadTemplatesDef() error {
	templatef := path.Join(p.template_dir, template_file)
	if _, err := os.Stat(templatef); err != nil {
		return fmt.Errorf("Unable to find templates definition file '%s'. %s.", templatef, err)
	}

	if d, err := ioutil.ReadFile(templatef); err != nil {
		return fmt.Errorf("Unable to load '%s'. %s.", templatef, err)
	} else {
		if err := yaml.Unmarshal(d, &p.templates_def); err != nil {
			return fmt.Errorf("Unable to load yaml file format '%s'. %s.", templatef, err)
		}
	}
	return nil
}

// Load list of files to copy and files to generate
func (p *JenkinsPlugin) DefineSources() error {
	// load all features
	p.yaml.Features = make([]string, 0, 5)
	for _, f := range p.templates_def.Features.Common {
		if v, err := Evaluate(f, p.SourceModel()); err != nil {
			return fmt.Errorf("Unable to evaluate '%s'. %s", f, err)
		} else {
			if v == "" {
				log.Printf("INFO! No feature defined with '%s'.", f)
				continue
			}
			f = v
		}
		if f != "" {
			p.yaml.Features = append(p.yaml.Features, f)
		}
	}

	if deploy_features, ok := p.templates_def.Features.Deploy[p.yaml.Deploy.Deployment.To]; ok {
		for _, f := range deploy_features {
			if f != "" {
				p.yaml.Features = append(p.yaml.Features, f)
			}
		}
	}

	// TODO: Load additionnal features from maintainer source path or file. This will permit adding more features and let the plugin manage generated path from update task.

	// Load all sources
	p.sources = make(map[string]TmplSource)
	p.templates = make(map[string]TmplSource)

	choose_file := func(file string, f TmplSource) error {
		if file == "" {
			return nil
		}
		if f.If != "" {
			if v, err := Evaluate(f.If, p.SourceModel()); err != nil {
				return fmt.Errorf("Unable to evaluate the '%s' condition '%s'. %s", file, f.If, err)
			} else {
				if v == "" || strings.ToLower(v) == "false" {
					log.Printf("Condition '%s' negative (false or empty). '%s' ignored.", f.If, file)
					return nil
				}
			}
		}
		if f.Template == "" {
			p.sources[file] = f
			log.Printf("SRC : selected: %s", file)
		} else {
			p.templates[file] = f
			log.Printf("TMPL: selected: %s", file)
		}
		return nil
	}

	for file, f := range p.templates_def.Sources.Common {
		if err := choose_file(file, f); err != nil {
			return err
		}
	}

	if deploy_sources, ok := p.templates_def.Sources.Deploy[p.yaml.Deploy.Deployment.To]; ok {
		for file, f := range deploy_sources {
			if err := choose_file(file, f); err != nil {
				return err
			}
		}
	}

	p.CleanSourceModel()

	return nil
}

// Generate read a source file and interpret it with text/template.
// It reads the source file, generate the final file and state if file generated has been updated or not.
//
// It permits to use another template tag to avoid conflict with the code to generate.
// This Tag have to define a pair string. Ex: "(())" the string is split in 2, to define the begin and end tag. ie begin="((", end="))"
// "((" is invalid because begin == end == "("
// "[[]" is also invalid, because the tag size is not divided per 2 (size is 3)
//
// When a different tag is used, any existing {{}} in source file will be kept after the template generation.
func (ts *TmplSource) Generate(tmpl_data interface{}, template_dir, dest_path, dest_name string) (updated bool, _ error) {
	src := path.Join(template_dir, ts.Template)
	dest := path.Join(dest_path, dest_name)
	parent := path.Dir(dest)

	if parent != "." {
		if _, err := os.Stat(parent); err != nil {
			os.MkdirAll(parent, 0755)
			updated = true
		}
	}

	var data string
	if b, err := ioutil.ReadFile(src); err != nil {
		return false, fmt.Errorf("Load issue. %s", err)
	} else {
		data = string(b)
	}

	if ts.Tag != "" {
		if v := len(ts.Tag); v < 4 {
			return false, fmt.Errorf("%s: tag template string must have at least 2 cars for 'begin' and 'end' tag. '%s' is too small", ts.Template, ts.Tag)
		}
		if v := len(ts.Tag); v%2 != 0 {
			return false, fmt.Errorf("%s: tag template string must define begin and end different tag, each of same size. Got %d", ts.Template, v)
		}
		tagSize := len(ts.Tag) / 2
		tag1 := ts.Tag[0:tagSize]
		tag2 := ts.Tag[tagSize:]

		if tag1 == tag2 {
			return false, fmt.Errorf("%s: tag template string (%s) must define different string for begin/end tag. Got begin='%s' vs end='%s'",
				ts.Template, ts.Tag, tag1, tag2)
		}

		if tag1 == "{{" || tag1 == "}}" {
			return false, fmt.Errorf("%s: begin tag '%s' cannot be '{{' or '}}'", ts.Template, tag1)
		}
		if tag2 == "{{" || tag2 == "}}" {
			return false, fmt.Errorf("%s: end tag '%s' cannot be '{{' or '}}'", ts.Template, tag2)
		}

		gotrace.Trace("Tag: begin='%s', end='%s'", tag1, tag2)

		replacer := map[string]string{
			"}}": tag1 + "`}}`" + tag2,
			"{{": tag1 + "`{{`" + tag2,
			tag1: "{{",
			tag2: "}}",
		}
		for _, tagSel := range []string{"{{", "}}", tag1, tag2} {
			data = strings.Replace(data, tagSel, replacer[tagSel], -1)
		}
	}

	data = strings.Replace(data, "}}\\\n", "}}", -1)

	t, err := template.New(src).Funcs(template.FuncMap{
		"lookup": lookup,
	}).Parse(data)
	if err != nil {
		return false, fmt.Errorf("Template issue. %s", err)
	}

	orig_md5, _ := md5sum(dest)
	final_md5_file := md5.New()

	if out, err := os.Create(dest); err != nil {
		return false, fmt.Errorf("Unable to create %s. %s", dest, err)
	} else {
		multi_write_file := io.MultiWriter(out, final_md5_file)
		if err := t.Execute(multi_write_file, tmpl_data); err != nil {
			return false, fmt.Errorf("Unable to interpret %s. %s", dest, err)
		}
		out.Close()
	}
	final_md5 := final_md5_file.Sum(nil)
	if orig_md5 != nil {
		updated = updated || !bytes.Equal(orig_md5, final_md5)
	} else {
		updated = true
	}

	if u, err := set_rights(dest, ts.Chmod); err != nil {
		return false, fmt.Errorf("%s", err)
	} else {
		updated = updated || u
	}
	return
}

func lookup(m map[string]interface{}, key string) (interface{}, error) {
	val, ok := m[key]
	if !ok {
		return nil, errors.New("missing key " + key)
	}
	return val, nil
}
