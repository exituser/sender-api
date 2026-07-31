#!/bin/sh
set -eu

# Creates the SES classic receipt rule used by the optional inbound worker.
# The script is intentionally fail-closed: an existing rule with different
# recipients/actions is never overwritten, and activation of a different
# active rule set requires an explicit confirmation variable.

: "${AWS_REGION:?AWS_REGION must be set}"
: "${INBOUND_DOMAIN:?INBOUND_DOMAIN must be set}"
: "${INBOUND_BUCKET:?INBOUND_BUCKET must be set}"
: "${INBOUND_TOPIC_ARN:?INBOUND_TOPIC_ARN must be set}"

rule_set_name=${INBOUND_RULE_SET_NAME:-sender-api-inbound}
rule_name=${INBOUND_RULE_NAME:-store-and-notify}
object_key_prefix=${INBOUND_OBJECT_KEY_PREFIX:-raw/}

rule_json=$(jq -n \
  --arg name "$rule_name" \
  --arg domain "$INBOUND_DOMAIN" \
  --arg bucket "$INBOUND_BUCKET" \
  --arg prefix "$object_key_prefix" \
  --arg topic "$INBOUND_TOPIC_ARN" \
  '{Name: $name, Enabled: true, ScanEnabled: true,
    Recipients: [$domain],
    Actions: [
      {S3Action: {BucketName: $bucket, ObjectKeyPrefix: $prefix}},
      {SNSAction: {TopicArn: $topic, Encoding: "UTF-8"}}
    ]}')

if ! aws ses describe-receipt-rule-set \
  --region "$AWS_REGION" \
  --rule-set-name "$rule_set_name" \
  >/dev/null 2>&1; then
  aws ses create-receipt-rule-set \
    --region "$AWS_REGION" \
    --rule-set-name "$rule_set_name" \
    >/dev/null
fi

existing_rule=''
if existing_rule=$(aws ses describe-receipt-rule \
  --region "$AWS_REGION" \
  --rule-set-name "$rule_set_name" \
  --rule-name "$rule_name" \
  --output json 2>/dev/null); then
  if ! printf '%s\n' "$existing_rule" | jq -e \
    --argjson expected "$rule_json" \
    '$expected.Enabled == .Rule.Enabled
     and $expected.ScanEnabled == .Rule.ScanEnabled
     and ($expected.Recipients == .Rule.Recipients)
     and ([.Rule.Actions[] | select(.S3Action.BucketName == $expected.Actions[0].S3Action.BucketName and .S3Action.ObjectKeyPrefix == $expected.Actions[0].S3Action.ObjectKeyPrefix)] | length) == 1
     and ([.Rule.Actions[] | select(.SNSAction.TopicArn == $expected.Actions[1].SNSAction.TopicArn and .SNSAction.Encoding == $expected.Actions[1].SNSAction.Encoding)] | length) == 1' \
    >/dev/null; then
    printf 'Receipt rule %s already exists but does not match the requested domain or destinations.\n' "$rule_name" >&2
    exit 1
  fi
else
  aws ses create-receipt-rule \
    --region "$AWS_REGION" \
    --rule-set-name "$rule_set_name" \
    --rule "$rule_json" \
    >/dev/null
fi

active_rule_set=$(aws ses describe-active-receipt-rule-set \
  --region "$AWS_REGION" \
  --query 'Metadata.Name' \
  --output text 2>/dev/null || true)
if [ "$active_rule_set" != "$rule_set_name" ]; then
  if [ "${CONFIRM_RECEIPT_RULE_ACTIVATION:-}" != "YES" ]; then
    printf 'Receipt rule is prepared but not active. Set CONFIRM_RECEIPT_RULE_ACTIVATION=YES to switch active mail routing.\n'
    exit 0
  fi
  aws ses set-active-receipt-rule-set \
    --region "$AWS_REGION" \
    --rule-set-name "$rule_set_name" \
    >/dev/null
fi

printf 'SES inbound receipt rule %s is configured in rule set %s.\n' "$rule_name" "$rule_set_name"
