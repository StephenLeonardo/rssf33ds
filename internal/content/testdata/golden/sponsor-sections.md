Queue depth is an incomplete measure of whether a background system is healthy. A queue with few items can still be stuck if consumers are failing, while a large queue may be expected during a planned batch. Track the age of the oldest item and the time required to complete work so alerts reflect customer impact.

Consumers should acknowledge a task only after its durable effects are complete. If the process crashes before acknowledgement, the queue can deliver the task again. Idempotent handlers make that retry safe, and a dead letter path gives operators a bounded place to investigate messages that repeatedly fail.

## Set an alert on age

Choose an age limit based on the workflow's service objective, then alert before customers are likely to notice a delay. During an incident, pause producers only when doing so is safe; otherwise preserve incoming work and add consumer capacity. After recovery, inspect retry and dead letter counts to confirm that the system has caught up.

## Review capacity regularly

Measure processing time under typical and peak loads. A slow downstream dependency can reduce consumer throughput even when the queue itself is operating correctly. Capacity planning should include that dependency and leave room for retries, deployments, and uneven traffic.

Alert thresholds need regular review as the workflow changes. A target that was reasonable for a small launch may be too slow after the queue becomes part of a customer-facing process. Use observed completion times and product expectations to keep alerts useful instead of noisy.

After a failure, compare the queue age with consumer errors and downstream latency. This helps determine whether additional workers will improve throughput or simply increase pressure on a failing dependency. A clear runbook should describe safe scaling steps and the point at which operators should stop accepting new work.
