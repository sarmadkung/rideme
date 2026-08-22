# 125 — Notification Templates & Localization

## Objective
Centralize message templates and support multilingual communication.

## Template
```text
template_id
channel
locale
version
subject
body
variables
status
```

## Variables
Example:
```text
{{order_number}}
{{driver_name}}
{{eta}}
{{support_reference}}
```

Templates must define allowed variables.

## Localization
Support locale fallback:
```text
user locale
↓
default locale
```

Do not concatenate localized messages in business logic.

## Versioning
A notification sent in the past must remain reproducible from its recorded template/version.

## Definition of Done
Content changes do not require redeploying core business services.
