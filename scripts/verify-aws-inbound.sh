#!/bin/sh
set -eu

: "${AWS_REGION:?AWS_REGION must be set}"
: "${INBOUND_DOMAIN:?INBOUND_DOMAIN must be set}"
: "${INBOUND_BUCKET:?INBOUND_BUCKET must be set}"
: "${INBOUND_TOPIC_ARN:?INBOUND_TOPIC_ARN must be set}"

rule_set_name=${INBOUND_RULE_SET_NAME:-sender-api-inbound}
rule_name=${INBOUND_RULE_NAME:-store-and-notify}

active_rule_set=$(aws ses describe-active-receipt-rule-set \
  --region "$AWS_REGION" \
  --query 'Metadata.Name' \
  --output text)
if [ "$active_rule_set" != "$rule_set_name" ]; then
  printf 'Active SES receipt rule set is %s; expected %s.\n' "$active_rule_set" "$rule_set_name" >&2
  exit 1
fi

rule_json=$(aws ses describe-receipt-rule \
  --region "$AWS_REGION" \
  --rule-set-name "$rule_set_name" \
  --rule-name "$rule_name" \
  --output json)

printf '%s\n' "$rule_json" | jq -e \
  --arg domain "$INBOUND_DOMAIN" \
  --arg bucket "$INBOUND_BUCKET" \
  --arg topic "$INBOUND_TOPIC_ARN" \
  '.Rule.Enabled == true
   and (.Rule.Recipients | index($domain) != null)
   and ([.Rule.Actions[] | select(.S3Action.BucketName == $bucket)] | length) == 1
   and ([.Rule.Actions[] | select(.SNSAction.TopicArn == $topic)] | length) == 1' \
  >/dev/null

printf 'SES inbound rule is active and matches domain, bucket, and SNS topic.\n'
