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

resource "skopeo2_copy" "default" {
  count = 1
  source {
    image                 = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/ecr-public/docker/library/busybox:uclibc"
    login_username        = "AWS"
    login_password_script = "aws --profile bsquare-jenkins2-moved ecr get-login-password --region=us-west-1"
    timeout               = 9
  }
  destination {
    image                 = "docker://329020582682.dkr.ecr.eu-west-2.amazonaws.com/copytest:latest-${count.index}"
    login_username        = "AWS"
    login_password_script = "aws ecr get-login-password --region=eu-west-2"
  }
  preserve_digests = true
  keep_image       = true
  copy_all_images  = true
  lifecycle {
    ignore_changes = [source.0.login_password_script, destination.0.login_password_script]
  }
}
