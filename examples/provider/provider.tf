terraform {
  required_providers {
    skopeo2 = {
      source  = "bsquare-corp/skopeo2"
      version = "~> 1.1.0"
    }
  }
}

provider "skopeo2" {
  source {
    login_username = "my-source-registry-username"
    login_password = "my-source-registry-password"
  }
  destination {
    login_username = "my-destination-registry-username"
    login_password = "my-destination-registry-password"
  }
}