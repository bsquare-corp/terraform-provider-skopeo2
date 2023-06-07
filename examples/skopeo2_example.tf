
terraform {
  required_providers {
    skopeo2 = {
      source  = "bsquare-corp/skopeo2"
      version = "0.0.6"
    }
  }

  required_version = ">= 0.15"
}


provider "skopeo2" {
}

resource "skopeo2_copy" "example" {
  source {
    image = "docker://foo:latest"
  }
  destination {
    image = "docker://bar:latest"
  }
  preserve_digests = true
}