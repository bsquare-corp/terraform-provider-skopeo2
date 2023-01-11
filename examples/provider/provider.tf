terraform {
  required_providers {
    aws = {
      source  = "bsquare-corp/skopeo2"
      version = "~> 0.0.0"
    }
  }
}


provider "skopeo2" {
}