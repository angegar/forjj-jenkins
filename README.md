# Forjj jenkins plugin

## Introduction

This plugin generates runnable code (deploy code) to add (create and maintain) a continuous integration component (Jenkins) to your forge.

By default, it implements latest **Jenkins 2.x** with **pipeline**, several **jenkins plugins**, **basic security rights** and a collection of scripts to enhance jenkins management from code perspective.

Mainly it offers:

- Add/Maintain new project repository to generate jobs (`projects`)
  (Jenkinsfile + JobsDSL).
- Add/Maintain Jenkins plugins/Jenkins features (`features`) (See [jenkins-install-inits(]https://github.com/forj-oss/jenkins-install-inits) and [jplugins](https://github.com/forj-oss/jplugins))
- Deploy Jenkins (master) on Docker/Swarm/UCP/Mesos/DCOS and scale it. (`applications/<instanceName>/deploy-to`)

## How to create your jenkins CI infrastructure for your organization?

Using forjj, it is quite easy.

1. Create a new jenkins instance in your Main Forjfile:

    ```yaml
    applications:
      <Instance Name>: # usually 'jenkins'
        type: ci
        driver: jenkins
    ```

2. Run `forjj update` (or `forjj create` when you create your forge, the first time.)
3. Run `forjj maintain dev` (`dev` is a deployment example name. See Forjfile for details)
4. Check at http://localhost:8080. 
 
   The admin password is `MyAdmin2016` by default.
  
   To change it, call `forjj secrets set app/<Instance Name>/admin-pwd`

By default, this will create a Jenkins container started in your docker host (Forjj DooD) and become accessible at http://localhost:8080

Behind the scene, forjj-jenkins generates a collection of deployment files in a new deployment repository from the [forjj-jenkins templates files](templates).

For details about how templates works or create your own templates, read [the Jenkins source Templates section](#jenkins-source-templates)

## Jenkins source templates

All jenkins source files are generated from a collection of source templates.

Forjj itself uses the templates stored under [templates](templates) to build and deliver itself.

When you start this forjj-jenkins component, it will use this [templates subdirectory](templates)

If this runnable forjj jenkins works for you, you will just need to set proper forjj-jenkins options in your Forjfile.

But you can create your own templates to deploy Jenkins in your company context.
To create your own templates files, set `applications/<instanceName>/source-templates` in your main Forjfile with the name of the template directory.
The template directory must be created in your infra repository under `apps/ci/<instanceName>/templates/<source-templates Name>`

At least, a `templates.yaml` must exist with:

```yaml
---
sources:
  common:
run_deploy:
```

This example will do nothing... So the next will be to ask forjj-jenkins to generate files for your deployment. 

Here is what you will need to do:

1. Set `applications/<instanceName>/deploy-to` if default `docker` does not make sense for you.
2. Add `<deploy-to Name>` under `sources`, `run_deploy` and optionally `build_deploy` of your templates.yaml
3. Declare the list of files to create/generate. Files are generated in your deployment repository under `<instance Name>/`
    
    create a description text section of the file to copy/generate to store few sub keys:
    - set `sources/<deploy-to Name or common>/template` with a relative path to the GO template source file. Destination file will be generated with the relative path under `<instance Name>/`
        `template` and `source` are exclusive. if you set both, an error will occur.
    - set `sources/<deploy-to Name or common>/source` with a relative path to the source file to copy. Destination file will be copied with the relative path under `<instance Name>/`
        `template` and `source` are exclusive. if you set both, an error will occur.
    - set `sources/<deploy-to Name or common>/chmod` to set file mode in octal representation. Default is 0644.
    - set `sources/<deploy-to Name or common>/tags` to a paired string to use as template tag. Used by `template`. Default is `{{}}`
        ex: Ansible use `{{}}` by jinja2 template mechanism. So, to avoid conflicts, you can set `#{{}}#` to interpret forjj-jenkins template tags.
    - set `sources/<deploy-to Name or common>/if` to determine if the file have to be copied or generated. 

      if `if` is set to a non empty string, the file will be copied/generated.
      The condition is made with GO template to output an empty string or not. ex: `"{{ index .Creds \"app-jenkins-aws-access-key\" }}"`
      The string can be interpreted by GO template to determine if the file needs to be copied/generated or not. For template data model see next section [template data model](#template_data_model).

4. Define build & deploy commands to execute
    - set `build_deploy/<deploy-to Name or common>/run` with a shell command to execute
    - set `build_deploy/<deploy-to Name or common>/run/environment` with a list of shell environment variable to define
      - under this section, set `<Your Environment Variable>/value` with the value to define. It can be interpreted by GO template with `{{}}`. For template data model see next section [template data model](#template_data_model).
      - under this section, set `<Your Environment Variable>/if` to define a condition for setting this environment variable.

        if `if` is set to a non empty string, the file will be copied/generated.

        The condition is made with GO template to output an empty string or not.
        
        ex: `"{{ lookup .Creds \"app-jenkins-aws-access-key\" }}"`
        
        For template data model see next section [template data model]

    - set `build_deploy/<deploy-to Name or common>/run/files` with a list of files to create
      - under this section, set `<Your relativePath to a file>/content` with the value to define. 

        It can be interpreted by GO template with `{{}}`. 

        For template data model see next section [template data model](#template_data_model).

      - under this section, set `<Your Environment Variable>/if` to define a condition for setting this environment variable.

        if `if` is set to a non empty string, the file will be copied/generated.

        The condition is made with GO template to output an empty string or not. 
        
        ex: `"{{ lookup .Creds \"app-jenkins-aws-access-key\" }}"`

        For template data model see next section [template data model]
      
      - under this section, set `<Your Environment Variable>/remove-when-done:` to false if you do not want to remove the file when shell command exits. Default is true.
      - under this section, set `<Your Environment Variable>/create-subdirs:` to true if you want to create relative sub directory if missing. Default is false.

Ex:

```yaml
---
sources:
  common:
    template: relativePath/myfile.txt # Template read from `<infra Repo>/apps/ci/<instanceName>/templates/<source-templates Name>/` and generated to `<deploy Repo>/<instanceName>/`
    if: "{{if eq .Source.Deploy.Ssl.Method \"manual\" }}true{{ end }}}}" # check ssl method string is equal to "manual" and print out "true" if the condition is true
    tags: "(())" # Use '(('  '))' as begin/end template tag.
    chmod: 0400 # Read mode for user only.
  myDeployToString:
    source: relative/path/to/myfile.copy # Simple copy from `<infra Repo>/apps/ci/<instanceName>/templates/<source-templates Name>/` to `<deploy Repo>/<instanceName>/`
    chmod: 0777
run_deploy:
  myDeployToString:
    run: "bin/build-ansible.sh && bin/build-jenkins.sh && bin/provision.sh"
    environment:
      DEV_USER: 
        value: "{{ .Env.Username }}"
    files:
      ansible/playbook/roles/jenkins-master-container/files/jenkins-creds.json:
        content: "{{ index .Creds \"app-jenkins-feature-credentials-json\" }}"
        if: "{{ index .Creds \"app-jenkins-feature-credentials-json\" }}"
        remove-when-done: false # Usually used for debugging case.
        create-subdirs: true # if `ansible/playbook/roles/jenkins-master-container/files` doesn't exist, create it before creating the file.
```

## Embedded forjj Jenkins template

Currently, the embedded jenkins template implements the following:

- A docker image built from `hub.docker.io/forjdevops/jenkins` [source](https://github.com/forj-oss/jenkins-ci)
- A collection of default features ([source](https://github.com/forj-oss/jenkins-install-inits))
  - Basic authentication (admin user with default password & anonymous has read access)
  - proxy setting (Set proxy from http_proxy env setting, found from the container)
  - seed-job (One job generated to populate the other collection of jobs/pipelines)
  - jenkins slave fixed port
  - pipeline
- A collection of additional features and templates to add for a dedicated deployment
- A list of predefined deployment. ie:
  - docker - To deploy to your local docker environment. (Default deployment)
  - ucp - To deploy to a UCP system. **NOT AVAILABLE**
  - marathon - To deploy to dcos/mesos marathon. **NOT AVAILABLE**

This list of elements are not exhaustive and can be updated time to time. Please refer to the (templates.yaml)[templates/templates.yaml] for latest updates.

## Forjfile plugin options

forjj-jenkins has several objects and that can be defined in your Forjfile.

For details, read [jenkins.yaml](jenkins.yaml)

**NOTE**: In short future, forjj will show up all plugins options with something like `forjj show`

## github upstream with pull-request flow setting.

The github integration will update your `<infra>/ci/<instance Name>` with the following code.

- `pipeline github` feature
- 1 Jobs DSL for each project identified under `<deploy Repo>/<instance Name>/jobs-dsl/*.groovy`

### Other SCM

Currently this jenkins Forjj plugin do not have any other upstream integration.
But this CI orchestrator has been designed to easily add a new one, like gitlab or other flows.
If you want to add you SCM/Jenkins integration, consider contribution to this repository.

For details on contribution, see CONTRIBUTION.md

## Contribution

Feel free to contribute and create a pull request or issue.

For details on contribution, see CONTRIBUTION.md

## Todo

Following list is not exhaustive and is given as an example. Check list of enhancement or issues in [forjj-jenkins repository](https://github.com/forj-oss/forjj-jenkins/issues)

- `applications/<instanceName>/source-templates` could define a git repo, so that we can share the template more easily.

Forj team
