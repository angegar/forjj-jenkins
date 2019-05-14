pipeline {
    agent any
    
     triggers {
        cron('0 0 * * 2') //At 00:00 on Thursday
    }
    
    stages {
        stage('prepare build environment') {
            steps {
                sh('''set +x ; source ./build-env.sh
                create-go-build-env.sh''')
            }
        }
        stage('Install dependencies') {
            steps {
                sh('''set +x ; source ./build-env.sh
                glide i''')
            }
        }
        stage('Build') {
            steps {
                sh('''set +x ; source ./build-env.sh
                go build''')
            }
        }
        stage('Tests') {
            steps {
                sh('''set +x ; source ./build-env.sh
                go test forjj-jenkins forjj-jenkins/reportlogs''')
            }
        }
        stage('Deploy') {
            when { branch 'master' }
            steps {
                withCredentials([
                usernamePassword(credentialsId: 'github-jenkins-cred', usernameVariable: 'GITHUB_USER', passwordVariable: 'GITHUB_TOKEN')]) {
                    withCredentials([
                    usernamePassword(credentialsId: 'dockerhub-cred', usernameVariable: 'DOCKERHUB_USERNAME', passwordVariable: 'DOCKERHUB_PASSWORD')]) {
                        sh('''set +x ; source ./build-env.sh
                        publish-alltags.sh latest
                        ''')
                    }
                }
            }
        }
    }

    post {
        success {
            deleteDir()
        }
    }
}
