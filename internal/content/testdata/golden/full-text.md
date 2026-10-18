## Abstract

Retail purchases can supplement clinical surveillance because changes in demand for some over-the-counter (OTC) products may appear before confirmed case reports. This article describes an early-stage concept for analysing aggregated GrabMart transaction data to identify unusual changes in product demand. The concept is not a deployed system, and it would require validation by public health experts, privacy review, and regulatory approval before any implementation.

## Introduction

Public health agencies use sources such as clinical case reports, laboratory results, and hospital records to monitor disease activity. Retail data may offer an additional signal when demand changes across groups of symptom-related products. It cannot establish a diagnosis or replace clinical evidence.

When using indirect signals, we should assess whether they remain relevant to the outcome we want to measure. This includes considering how changes in user behaviour could affect their interpretation. A retail-based signal can experience similar drift, selection bias, and false alarms. Any useful design must therefore combine statistical validation with careful privacy and governance controls.

## Background

Syndromic surveillance tracks patterns that may indicate emerging illness before diagnoses are confirmed. Retail surveillance applies this idea to changes in demand for products associated with symptoms, such as fever remedies or cough medicines.

The proposed method groups eligible products into broad symptom classes and measures changes in aggregate demand over time. An anomaly score can indicate that observed demand differs from an expected seasonal pattern. The score is a statistical signal, not an estimate of how many people have a disease.

The design uses the following terms:

## Proposed solution

The proposed pipeline would process only transactions that meet documented eligibility rules. It would map eligible SKUs to symptom classes, aggregate counts across approved time periods and geographic areas, and compare each series with an appropriate baseline. Public health experts would review the resulting Early Outbreak Detection Index alongside independent evidence.

The method deliberately excludes identity linkage and individual-level health inference. It also avoids using a single purchase to classify a customer, target an advertisement, adjust an insurance decision, or change access to a product.

### Conceptual data flow

```
Eligible transactions | vReviewed SKU-to-symptom mapping | vAggregate by approved time period and geographic area | vCompare with seasonal and historical baselines | vPrivacy and quality checks | vAggregate signal for authorised public health evaluation
```

### Analysis and validation

Several statistical methods could model expected demand, including seasonal time-series models and spatial scan statistics. Model selection should follow retrospective evaluation against approved public health data. Evaluation should measure sensitivity, false-alert rate, calibration, geographic stability, and performance during seasonal changes.

The system would also need ongoing monitoring. Product catalogues change, purchasing behaviour shifts, promotions affect demand, and access to GrabMart varies across populations and locations. These factors can produce anomalies unrelated to disease activity.

### Privacy and governance

Privacy protection starts with data minimisation. The analysis should use only eligible product categories, coarse geographic areas, and aggregate time windows approved for the stated public health purpose. It should not expose raw transactions or personally identifiable information (PII).

The release process could apply minimum-count thresholds and differential privacy (DP), subject to a documented privacy analysis. Privacy specialists would need to set and review any privacy budget. Access controls, retention limits, audit logs, and contractual restrictions would govern any approved data exchange.

An independent review should cover privacy law, health-related inference, fairness, scientific validity, and the risk of harm from false alerts. An authorised public health body, rather than an automated model, should decide whether and how to communicate a possible outbreak.

## Potential applications

If research validates the signal and the required approvals are in place, aggregate trends could support:

- public health research alongside established surveillance sources;
- operational planning for broad categories of health supplies; and
- evaluation of whether retail demand contributes useful evidence during an emerging event.

These applications should use aggregate trends only. The signal should not drive personalised medical advice, individual targeting, insurance decisions, or automatic changes to medicine search rankings.

## Limitations

Retail demand is an indirect and incomplete measure of illness. Purchases may reflect promotions, weather, media coverage, household stocking, or changes in product availability. GrabMart customers are not representative of the full population, and the available product mix differs by market.

Mapping a product to a symptom class is also uncertain because many products have several uses. Aggregation and privacy controls reduce disclosure risk but can weaken statistical utility. These limitations require local validation and documented thresholds before anyone interprets the index.

If this mission speaks to you, join our team today!
