resource "skopeo2_copy" "example" {
  source {
    image         = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/blib/deployed-container-scanner-trivy:latest"
    login_script  = "aws --profile $MY_PROFILE ecr get-login-password --region us-west-1 | docker login --username AWS --password-stdin 753989949864.dkr.ecr.us-west-1.amazonaws.com"
    login_retries = 3
    login_environment = {
      MY_PROFILE = "bsquare-jenkins2"
    }
    login_script_interpreter = ["/bin/sh", "-c"]
  }
  destination {
    image         = "docker://329020582682.dkr.ecr.us-west-2.amazonaws.com/blib/deployed-container-scanner-trivy:latest"
    login_script  = "aws --profile $MY_PROFILE ecr get-login-password --region us-west-1 | docker login --username AWS --password-stdin 753989949864.dkr.ecr.us-west-1.amazonaws.com"
    login_retries = 3
    login_environment = {
      MY_PROFILE = "bsquare-jenkins2"
    }
    login_script_interpreter = ["/bin/sh", "-c"]
  }
  insecure         = false
  preserve_digests = true
  retries          = 3
  retry_delay      = 10
  additional_tags  = ["deployed-container-scanner-trivy:my-tag"]
  keep_image       = false
}
