terraform {
  required_providers {
    skopeo2 = {
      source  = "bsquare-corp/skopeo2"
      version = ">= 1.1.0"
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
}

data "aws_ecr_authorization_token" "source" {
  provider = aws.source
}

data "aws_ecr_authorization_token" "dest" {
  provider = aws.dest
}

provider "skopeo2" {
  alias = "bill"

  source {
    login_username = data.aws_ecr_authorization_token.source.user_name
    login_password = data.aws_ecr_authorization_token.source.password
  }
  destination {
    login_username = data.aws_ecr_authorization_token.dest.user_name
    login_password = data.aws_ecr_authorization_token.dest.password
  }
}

provider "skopeo2" {
  alias = "bob"

  source {
    login_username = data.aws_ecr_authorization_token.source.user_name
    login_password = data.aws_ecr_authorization_token.source.password
  }
  destination {
    login_username = data.aws_ecr_authorization_token.dest.user_name
    login_password = data.aws_ecr_authorization_token.dest.password
  }
}

resource "skopeo2_copy" "default" {
  provider = skopeo2.bill

  count = 5

  source_image      = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/ecr-public/docker/library/busybox:uclibc"
  destination_image = "docker://329020582682.dkr.ecr.eu-west-2.amazonaws.com/copytest:latest-${count.index}"

  preserve_digests = true
  keep_image       = true
  copy_all_images  = true
}

resource "skopeo2_copy" "default2" {
  provider = skopeo2.bob

  count = 5

  source_image      = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/ecr-public/docker/library/busybox:uclibc"
  destination_image = "docker://329020582682.dkr.ecr.eu-west-2.amazonaws.com/copytest:latest-${count.index}"

  preserve_digests = true
  keep_image       = true
  copy_all_images  = true
}
