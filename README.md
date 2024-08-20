# Terraform Provider skopeo2 (Terraform Plugin SDK)

Reference to earlier provider https://registry.terraform.io/providers/abergmeier/skopeo written by
abergmeier that this project is based on.

_This repository is built on the [Terraform Plugin SDK](https://github.com/hashicorp/terraform-plugin-sdk) template. The template repository built on the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) can be found at [terraform-provider-skopeo2-framework](https://github.com/bsquare-corp/terraform-provider-skopeo2-framework). See [Which SDK Should I Use?](https://www.terraform.io/docs/plugin/which-sdk.html) in the Terraform documentation for additional information._

This repository is a [Terraform](https://www.terraform.io) provider. It contains:

 - A resource, and a data source (`internal/provider/`),
 - Examples (`examples/`) and generated documentation (`docs/`),
 - Miscellaneous meta files.
 
These files extend the boilerplate code provided by the template needed for the skopeo2 Terraform provider. Tutorials for creating Terraform providers can be found on the [HashiCorp Learn](https://learn.hashicorp.com/collections/terraform/providers) platform.

Please see the [GitHub template repository documentation](https://help.github.com/en/github/creating-cloning-and-archiving-repositories/creating-a-repository-from-a-template) for how this repository was created from the template on GitHub.

This provider has been [published on the Terraform Registry](https://www.terraform.io/docs/registry/providers/publishing.html) so that others can use it.


## Requirements

-	[Terraform](https://www.terraform.io/downloads.html) >= 0.13.x
-	[Go](https://golang.org/doc/install) >= 1.19

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command: 
```sh
$ go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using the provider

The provider terraform-provider-skopeo2 will need to be copied from `$GOPATH/bin` to your plugins directory, 
normally here `~/.terraform.d/plugins/terraform.bsquare.com/bsquare-corp/skopeo2/1.1.0/linux_amd64/terraform-provider-skopeo2`
Then `terraform init` will copy the plugin into the local `.terraform/providers`. Note, if the plugin already exists 
in the local `.terraform/providers` then it will not be copied and terraform will complain if the binary has changed.

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```sh
$ make testacc
```

## Referencing Custom containers/image/v5 Dependency

Skopeo2 provider is built on a custom version of the containers/image code here: https://github.com/bsquare-corp/image/tree/creds-store-fix
In order to replace references to `containers/image/v5` the following command adds (or updates) a `replace` directive in the `go.mod` file:
```sh
$ go mod edit -replace github.com/containers/image/v5=github.com/bsquare-corp/image/v5@creds-store-fix
```
