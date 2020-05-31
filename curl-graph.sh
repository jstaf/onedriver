#!/bin/bash

TOKEN=$(jq -r .access_token $1)
ENDPOINT=$2
shift 2

curl -s -H "Authorization: bearer $TOKEN" $@ "https://graph.microsoft.com/v1.0$ENDPOINT"

