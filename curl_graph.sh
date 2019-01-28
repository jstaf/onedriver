#!/usr/bin/env bash

AUTH_TOKEN=$(jq -r .access_token $1)
curl --header "Authorization: bearer $AUTH_TOKEN" https://graph.microsoft.com/v1.0/$2 2> /dev/null | jq
