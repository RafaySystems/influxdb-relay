  pipeline {
    environment {
      registryCredential = 'rcloud_user_registry.stage.rafay.cloud'
      registryUrl= 'https://registry.dev.rafay-edge.net'
      dockerImage = ''
    }
    agent any
    stages {
      stage ('SonarQube Analysis') {
        environment {
          scannerHome = tool 'SonarScanner'
        }
        steps {
          withSonarQubeEnv('SonarQube-Prod') {
            sh "${scannerHome}/bin/sonar-scanner"
          }
        }
      }
      stage ('Quality Check') {
        steps {
          script {
            timeout (time: 5, unit: "MINUTES") {
              if ( waitForQualityGate().status != 'OK' ) {
                error "Pipeline Aborted"
              }
            }
          }
        }
      }
      stage('Building image') {
        steps{
          script {
            tag = "${env.GIT_BRANCH}"
            tags = tag.split("/")
            tag = tags[tags.size() - 1] + "-" + "${env.BUILD_NUMBER}"
            //if (tags.size() == 3) {
            //  tag = tags[2]
            //} else {
            //  tag = tags[1]
            //}
            //tag = tag + "-" + "${env.BUILD_NUMBER}"
            //dockerImage = docker.build("registry.dev.rafay-edge.net:5000/rafay/cluster-scheduler:" + tag, "--build-arg BUILD_USR=${env.BUILD_USER} --build-arg BUILD_PWD=${env.BUILD_PASSWORD} .")
            withCredentials([usernamePassword(credentialsId: 'jenkinsrafaygithub', passwordVariable: 'passWord', usernameVariable: 'userName')]) {
              docker.withRegistry(registryUrl, registryCredential) {
                withEnv(["TAG=${tag}"]) {
                  sh("docker buildx create --name influx --use")
                  sh('docker buildx build --provenance=false --build-arg BUILD_USR=$userName --build-arg BUILD_PWD=$passWord --tag registry.dev.rafay-edge.net/rafay/influxdb-relay:"$TAG" --load -f ./Dockerfile .')
                }
              }
          }
        }
      }
      }
      stage('Pushning image') {
        steps{
          script {
            tag = "${env.GIT_BRANCH}"
            tags = tag.split("/")
            tag = tags[tags.size() - 1] + "-" + "${env.BUILD_NUMBER}"
            withCredentials([usernamePassword(credentialsId: 'jenkinsrafaygithub', passwordVariable: 'passWord', usernameVariable: 'userName')]) {
              docker.withRegistry(registryUrl, registryCredential) {
                withEnv(["TAG=${tag}"]) {
                  sh("docker buildx create --name influx --use")
                  sh('docker buildx build --platform=linux/arm64,linux/amd64 --provenance=false --build-arg BUILD_USR=$userName --build-arg BUILD_PWD=$passWord --tag registry.dev.rafay-edge.net/rafay/influxdb-relay:"$TAG" --push -f ./Dockerfile .')
                }
              }
            }

              // docker.withRegistry(registryUrl, registryCredential ) {
              // dockerImage.push()
              // println dockerImage.imageName()
              // println dockerImage.id
              // DOCKER_IMAGE = dockerImage.id
              // sh("docker rmi ${DOCKER_IMAGE}")
              // }
          }
        }
      }
    }
    post {
      success {
        slackSend channel: "#build",
        color: 'good',
        message: "Build ${currentBuild.fullDisplayName} completed successfully."
      }
      failure {
        slackSend channel: "#build",
        color: 'RED',
        message: "Attention ${env.JOB_NAME} ${env.BUILD_NUMBER} has failed."
      }
      always {
        script {
          tag = "${env.GIT_BRANCH}"
          tags = tag.split("/")
          tag = tags[tags.size() - 1] + "-" + "${env.BUILD_NUMBER}"
          withEnv(["TAG=${tag}"]) {
            sh('docker rmi registry.dev.rafay-edge.net/rafay/influxdb-relay:"$TAG"')
          }
        }
      }
    }
}
