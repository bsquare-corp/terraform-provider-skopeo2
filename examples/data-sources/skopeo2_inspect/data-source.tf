provider "aws" {
  region = "eu-west-1"
}

data "aws_ecr_authorization_token" "default" {}

provider "skopeo2" {
  source {
    login_username = data.aws_ecr_authorization_token.default.user_name
    login_password = data.aws_ecr_authorization_token.default.password
  }
}

resource "skopeo2_inspect" "default" {
  source_image = "docker://000000000000.dkr.ecr.eu-west-1.amazonaws.com/my-image:latest"
}
