resource "skopeo2_copy" "external-script-login" {
  source {
    image         = "docker://000000000000.dkr.ecr.eu-west-1.amazonaws.com/my-image:latest"
    login_script  = "aws ecr get-login-password --region eu-west-1 | docker login --username AWS --password-stdin 000000000000.dkr.ecr.eu-west-1.amazonaws.com"
    login_retries = 3
    login_environment = {
      AWS_PROFILE = "default"
    }
    login_script_interpreter = ["/bin/sh", "-c"]
  }
  destination {
    image         = "docker://111111111111.dkr.ecr.us-west-2.amazonaws.com/my-image:latest"
    login_script  = "aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin 111111111111.dkr.ecr.us-west-2.amazonaws.com"
    login_retries = 3
    login_environment = {
      AWS_ACCESS_KEY_ID     = "AKIAIOSFODNN7EXAMPLE"
      AWS_SECRET_ACCESS_KEY = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
      AWS_DEFAULT_REGION    = "us-west-2"
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

resource "skopeo2_copy" "internal-login-external-script-for-password" {
  source {
    image                 = "docker://000000000000.dkr.ecr.eu-west-1.amazonaws.com/my-image:latest"
    login_username        = "AWS"
    login_password_script = "aws ecr get-login-password --region eu-west-1"
    login_retries         = 3
    login_environment = {
      AWS_PROFILE = "default"
    }
    login_script_interpreter = ["/bin/sh", "-c"]
  }
  destination {
    image                 = "docker://111111111111.dkr.ecr.us-west-2.amazonaws.com/my-image:latest"
    login_username        = "AWS"
    login_password_script = "aws ecr get-login-password --region us-west-2"
    login_retries         = 3
    login_environment = {
      AWS_ACCESS_KEY_ID     = "AKIAIOSFODNN7EXAMPLE"
      AWS_SECRET_ACCESS_KEY = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
      AWS_DEFAULT_REGION    = "us-west-2"
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

provider "aws" {
  alias  = "source"
  region = "eu-west-1"
}

provider "aws" {
  alias  = "dest"
  region = "us-west-2"

  assume_role {
    role_arn = "arn:aws:iam::111111111111:role/dest-access"
  }
}

data "aws_ecr_authorization_token" "source" {
  provider = aws.source
}

data "aws_ecr_authorization_token" "dest" {
  provider = aws.dest
}

provider "skopeo2" {
  source {
    login_username = data.aws_ecr_authorization_token.source.user_name
    login_password = data.aws_ecr_authorization_token.source.password
  }
  destination {
    login_username = data.aws_ecr_authorization_token.dest.user_name
    login_password = data.aws_ecr_authorization_token.dest.password
  }
}

resource "skopeo2_copy" "internal-login" {
  source_image      = "docker://000000000000.dkr.ecr.eu-west-1.amazonaws.com/my-image:latest"
  destination_image = "docker://111111111111.dkr.ecr.us-west-2.amazonaws.com/my-image:latest"

  insecure         = false
  preserve_digests = true
  retries          = 3
  retry_delay      = 10
  additional_tags  = ["my-image:my-tag"]
  keep_image       = false
}
