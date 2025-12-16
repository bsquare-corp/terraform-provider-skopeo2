1. Build the provider locally:

```shell
cd ../..
go build
```

2. Set the Terraform config to point to the locally built provider:

```shell
export TF_CLI_CONFIG_FILE=`pwd`/dev.tfrc
```

3. Initialise Terraform

```shell
 TF_LOG_PROVIDER=DEBUG terraform init -upgrade
```

4. Plan,Apply fill your boots.