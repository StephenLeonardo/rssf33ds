A request rarely stays inside one service. It may pass through a gateway, an application server, a queue, and several downstream APIs before a response returns to the caller. Looking at each service in isolation can make a slow operation appear healthy, so tracing records the path taken by one request across those boundaries.

A trace is made of spans. Each span describes an operation, its start and end time, and its relationship to a parent operation. Propagating trace context in request headers lets independently deployed services contribute spans to the same trace. Instrumentation should preserve existing context and create a new trace only when no valid context is present.

Useful span attributes describe an operation without copying sensitive payloads. Service name, route template, status, and a stable resource identifier often answer operational questions. High-cardinality values such as raw URLs or user identifiers can make storage expensive and may expose private information.

Sampling controls how much trace data is retained. Head based sampling makes an early decision and has predictable cost, while tail based sampling can retain unusual slow or failed traces after observing their outcome. Either approach needs representative coverage and clear rules for errors.

Tracing works best alongside metrics and logs. Metrics reveal patterns across many requests, traces show the sequence behind one request, and logs provide event details at a chosen point. Shared identifiers connect these views and give responders a practical route from an alert to the service and operation responsible for a delay.
