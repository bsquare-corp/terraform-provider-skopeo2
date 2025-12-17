terraform {
  required_providers {
    skopeo2 = {
      source  = "bsquare-corp/skopeo2"
      version = ">= 1.1.0"
    }
    aws = {
    }
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.0"
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

locals {
  registry_address = "329020582682.dkr.ecr.eu-west-2.amazonaws.com"
}

provider "docker" {
  registry_auth {
    address  = local.registry_address
    username = data.aws_ecr_authorization_token.dest.user_name
    password = data.aws_ecr_authorization_token.dest.password
  }
}

provider "skopeo2" {
  alias = "bill"

  source {
    login_username = data.aws_ecr_authorization_token.source.user_name
    login_password = data.aws_ecr_authorization_token.source.password
  }
  destination {
    login_username        = "AWS"
    login_password_script = "aws ecr get-login-password --region eu-west-2"
    login_retries         = 3
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

variable "image_name" {
  type    = string
  default = "copytest-src"
}

variable "image_tag" {
  type    = string
  default = "latest"
}

resource "local_file" "text_file" {
  filename = "${path.module}/build/hello.txt"
  content  = "Hello from a Terraform-built Docker image. MODD"
}

resource "local_file" "dockerfile" {
  filename = "${path.module}/build/Dockerfile"
  content  = <<EOF
FROM alpine:3.20
WORKDIR /app2
COPY hello.txt /app/hello.txt
CMD ["cat", "/app/hello.txt"]
EOF
  depends_on = [
    local_file.text_file,
  ]
}

resource "docker_image" "textfile_image" {
  name = "${local.registry_address}/${var.image_name}:${var.image_tag}"

  build {
    context    = "${path.module}/build"
    dockerfile = "Dockerfile"
  }

  depends_on = [
    local_file.text_file,
    local_file.dockerfile
  ]
}

resource "aws_ecr_repository" "this" {
  name                 = var.image_name
  image_tag_mutability = "MUTABLE"
}

resource "docker_registry_image" "textfile_image" {
  name = docker_image.textfile_image.name

  depends_on = [
    aws_ecr_repository.this,
    local_file.text_file,
    local_file.dockerfile
  ]
}

output "image_reference" {
  description = "Pushed Docker image reference"
  value       = docker_registry_image.textfile_image.name
}

resource "aws_ecr_repository" "bill" {
  provider             = aws.dest
  name                 = "copytest-bill"
  image_tag_mutability = "MUTABLE"
}

resource "skopeo2_copy" "default" {
  provider = skopeo2.bill

  count = 5

  source_image = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/ecr-public/docker/library/busybox:uclibc"
  //source_image      = "docker://${docker_registry_image.textfile_image.name}"
  destination_image = "docker://329020582682.dkr.ecr.eu-west-2.amazonaws.com/copytest-bill:latest-${count.index}"

  preserve_digests = true
  keep_image       = true
  copy_all_images  = true

  depends_on = [
    aws_ecr_repository.bill
  ]
}

resource "aws_ecr_repository" "bob" {
  provider             = aws.dest
  name                 = "copytest-bob"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
}

resource "aws_ecr_repository" "bob-mov" {
  provider             = aws.dest
  name                 = "copytest-bob-mov"
  image_tag_mutability = "MUTABLE"
  force_delete         = true
}

resource "skopeo2_copy" "default2" {
  provider = skopeo2.bob

  count = 5

  //source_image      = "docker://753989949864.dkr.ecr.us-west-1.amazonaws.com/ecr-public/docker/library/busybox:uclibc"
  source_image      = "docker://${docker_registry_image.textfile_image.name}"
  destination_image = "docker://329020582682.dkr.ecr.eu-west-2.amazonaws.com/copytest-bob-mov:latest-${count.index}"

  preserve_digests = true
  keep_image       = true
  copy_all_images  = true

  depends_on = [
    aws_ecr_repository.bob-mov
  ]
}
