#!/bin/bash

echo calling src_login_password.sh >&2

aws --profile bsquare-jenkins2 ecr get-login-password --region=us-west-1