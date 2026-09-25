# event-producer-consumer-alignment

## Focus
Webhook registrations, event emitters, and downstream webhook filters or
workers must maintain strict event type alignment. Only subscribe to events
that downstream workers actually consume (e.g. webhook-filter consumes Job Hook,
Push Hook, Note Hook; do not register merge_requests_events or pipeline_events
unless handlers are implemented).

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/event-alignment
