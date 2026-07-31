# AWS inbound messaging

This stack provisions the low-volume inbound path:

```text
SES receipt rule → S3 (raw email)
                 → SNS → SQS → sender-api worker → PostgreSQL
```

The current runtime uses `AWS_SES_CONFIGSET=default`. Create that named SES
configuration set once before deploying the application; it is intentionally
not managed by this stack so an existing configuration set is never replaced
or adopted implicitly. Event destinations for outbound delivery callbacks are
also intentionally not enabled until a public callback URL is selected.

The template deliberately does not create an SES receipt rule. A receipt rule
contains the verified recipient domain and is mail-flow configuration, so it is
applied only after confirming the domain and the desired retention period.

## Provision the transport

The AWS identity used for deployment needs permission to create and manage the
S3 bucket, SNS topic/subscription/policy, and SQS queues/policy. Use the normal
AWS credential provider chain; never commit credentials or copy them into this
directory.

```bash
aws sesv2 create-configuration-set \
  --region "${AWS_REGION}" \
  --configuration-set-name default
```

If the configuration set already exists, this command can be skipped.

```bash
aws cloudformation deploy \
  --region "${AWS_REGION}" \
  --stack-name sender-api-messaging \
  --template-file infra/aws/messaging.yaml \
  --parameter-overrides \
    InboundBucketName="${INBOUND_S3_BUCKET:-sender-api-inbound}" \
    InboundTopicName="sender-api-inbound" \
    InboundQueueName="sender-api-inbound" \
    InboundDeadLetterQueueName="sender-api-inbound-dlq" \
    RawEmailRetentionDays="30"

aws cloudformation describe-stacks \
  --region "${AWS_REGION}" \
  --stack-name sender-api-messaging \
  --query 'Stacks[0].Outputs'
```

Keep the returned `InboundQueueUrl` and `InboundTopicArn` private to the
deployment environment. Set them in `.env` or the container secret store:

```dotenv
INBOUND_S3_BUCKET=sender-api-inbound
INBOUND_SQS_QUEUE_URL=https://sqs.<region>.amazonaws.com/<account>/sender-api-inbound
INBOUND_SNS_TOPIC_ARN=arn:aws:sns:<region>:<account>:sender-api-inbound
```

The application verifies the SNS signature and exact topic ARN. The worker
long-polls SQS and deletes a message only after the database write succeeds;
failed messages are retried and eventually moved to the DLQ.

## Create and activate the SES receipt rule

Use the already verified domain, or pass a specific verified domain after
checking it in SES. The correct SES receiving action is `SnsAction`; SNS then
delivers the notification to SQS. Do not use a fictitious SES `SqsAction`.

```bash
aws ses create-receipt-rule-set \
  --region "${AWS_REGION}" \
  --rule-set-name sender-api-inbound

aws ses create-receipt-rule \
  --region "${AWS_REGION}" \
  --rule-set-name sender-api-inbound \
  --rule '{
    "Name": "store-and-notify",
    "Enabled": true,
    "ScanEnabled": true,
    "Recipients": ["<verified-domain>"],
    "Actions": [
      {"S3Action": {"BucketName": "sender-api-inbound", "ObjectKeyPrefix": "raw/"}},
      {"SNSAction": {"TopicArn": "<inbound-topic-arn>", "Encoding": "UTF-8"}}
    ]
  }'

aws ses set-active-receipt-rule-set \
  --region "${AWS_REGION}" \
  --rule-set-name sender-api-inbound
```

The domain must have MX records pointing to the SES inbound SMTP endpoint for
the selected region. SES sandbox restrictions still apply to sending; inbound
receiving and DNS are separate checks.

## Rollout checks

After updating the API and worker environment, verify:

```bash
docker compose config --quiet
docker compose up -d --build api worker
docker compose ps
docker compose logs --tail=100 worker
```

Send a test message to the configured inbound address and confirm that the
message appears in `/api/v1/inbound`, the SQS message is acknowledged, and the
S3 object is created under `raw/`. Do not use a production mailbox until the
receipt rule recipient and 30-day raw-email retention are confirmed.
