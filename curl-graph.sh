#!/bin/bash

TOKEN=$(jq -r .access_token ~/.cache/onedriver/auth_tokens.json)
curl -H "Authorization: bearer $TOKEN" "https://graph.microsoft.com/v1.0$1"

