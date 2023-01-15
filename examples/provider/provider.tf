terraform {
  required_providers {
    skopeo2 = {
      source  = "bsquare-corp/skopeo2"
      version = "~> 0.0.0"
    }
  }
}


provider "skopeo2" {
}