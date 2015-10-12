# Flywheel

Flywheel is a HTTP proxy which starts and stops EC2 instances sitting behind
it.

Other solutions stop and start instances on a schedule to reduce AWS EC2 costs.
While this works well when resources are used regularly, it is less than ideal
when resources are unused for weeks or months at a time.

Flywheel will automatically stop its instances when no requests have been
received for a period of time.

Requests made while powered down will be served a "Currently powered down,
click here to start" style page.

