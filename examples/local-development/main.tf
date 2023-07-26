terraform {
  required_providers {
    skopeo2 = {
      source  = "bsquare-corp/skopeo2"
      version = ">= 0.0.7"
    }
    aws = {
    }
  }

  required_version = ">= 0.15"
}

provider "aws" {
  alias  = "source"
  region = "us-west-1"

  assume_role {
    role_arn = "arn:aws:iam::753989949864:role/qa-cross-account-access"
  }
}

provider "aws" {
  alias  = "dest"
  region = "eu-west-2"

  assume_role {
    role_arn = "arn:aws:iam::753989949864:role/qa-cross-account-access"
  }
}

data "aws_ecr_authorization_token" "source" {
  provider = aws.source
}

data "aws_ecr_authorization_token" "dest" {
  provider = aws.dest
}

resource "skopeo2_copy" "default" {
  count = 1
  source {
    image          = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/ecr-public/docker/library/busybox:uclibc"
    login_username = data.aws_ecr_authorization_token.source.user_name
    login_password = data.aws_ecr_authorization_token.source.password
  }
  destination {
    image          = "docker://329020582682.dkr.ecr.eu-west-2.amazonaws.com/copytest:latest-${count.index}"
    login_username = data.aws_ecr_authorization_token.dest.user_name
    login_password = data.aws_ecr_authorization_token.dest.password
  }
  docker_digest    = "bob"
  preserve_digests = true
  keep_image       = true
  copy_all_images  = true
}
