resource "skopeo2_copy" "example-1" {
  source {
    image             = "docker://000000000000.dkr.ecr.us-west-1.amazonaws.com/my-image:latest"
    login_script      = "aws --profile $MY_PROFILE ecr get-login-password --region us-west-1 | docker login --username AWS --password-stdin 000000000000.dkr.ecr.us-west-1.amazonaws.com"
    login_retries     = 3
    login_environment = {
      MY_PROFILE = "default"
    }
    login_script_interpreter = ["/bin/sh", "-c"]
  }
  destination {
    image             = "docker://111111111111.dkr.ecr.us-west-2.amazonaws.com/my-image:latest"
    login_script      = "aws --profile $MY_PROFILE ecr get-login-password --region us-west-1 | docker login --username AWS --password-stdin 111111111111.dkr.ecr.us-west-2.amazonaws.com"
    login_retries     = 3
    login_environment = {
      MY_PROFILE = "default"
    }
    login_script_interpreter = ["/bin/sh", "-c"]
  }
  insecure         = false
  preserve_digests = true
  retries          = 3
  retry_delay      = 10
  additional_tags  = ["my-image:my-tag"]
  keep_image       = false
}

resource "skopeo2_copy" "example-2" {
  source {
    image                 = "docker://000000000000.dkr.ecr.us-west-1.amazonaws.com/my-image:latest"
    login_username        = "AWS"
    login_password_script = "aws --profile $MY_PROFILE ecr get-login-password --region us-west-1"
    login_retries         = 3
    login_environment     = {
      MY_PROFILE = "default"
    }
    login_script_interpreter = ["/bin/sh", "-c"]
  }
  destination {
    image                 = "docker://111111111111.dkr.ecr.us-west-2.amazonaws.com/my-image:latest"
    login_username        = "AWS"
    login_password_script = "aws --profile $MY_PROFILE ecr get-login-password --region us-west-2"
    login_retries         = 3
    login_environment     = {
      MY_PROFILE = "default"
    }
    login_script_interpreter = ["/bin/sh", "-c"]
  }
  insecure         = false
  preserve_digests = true
  retries          = 3
  retry_delay      = 10
  additional_tags  = ["my-image:my-tag"]
  keep_image       = false
}
