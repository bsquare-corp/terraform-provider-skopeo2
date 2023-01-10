
terraform {
  required_providers {
    skopeo2 = {
      source  = "registry.terraform.io/bsquare-corp/skopeo2"
      version = "~> 0.0.0"
    }
  }

  required_version = ">= 0.15"
}


provider "skopeo2" {
}

resource "skopeo2_copy" "example" {
  source_image = "docker://foo:latest"
  destination_image = "docker://bar:latest"
  preserve_digests = true
}