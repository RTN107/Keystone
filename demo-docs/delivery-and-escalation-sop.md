# Delivery and Escalation SOP

This procedure governs how Delivery runs a client engagement from discovery to handover, and how it responds when a live system stops working as agreed. It applies to every engineer, delivery lead and account owner. All times are IST (UTC+5:30) unless stated. If a client's contract disagrees with a target here, the contract wins. The Delivery Director owns this document and reviews it each April, at the start of the financial year.

## Engagement lifecycle

Every engagement moves through six stages in order: discovery, design, build, UAT, hypercare and handover. Each stage has entry criteria that must be true before it starts and exit criteria that must be true before it closes. The delivery lead records the date each stage opens and closes in the engagement log. Skipping or merging a stage needs the Delivery Director's written approval, and the reason goes in the log. The sections below set out each stage.

## Stage 1: Discovery

Entry: a signed statement of work and a named sponsor on the client side.
Work: stakeholder interviews, an inventory of the data and systems involved, and success measures agreed in writing.
Exit: a discovery report the sponsor has acknowledged, a risk list with owners, and a named client contact for every system we will touch. Discovery normally takes two to four weeks.

## Stage 2: Design

Entry: discovery report acknowledged by the sponsor.
Work: architecture, data flows, model and tooling choices, a security and data handling review, and a test plan.
Exit: a design document signed off by the client sponsor and by our solution architect, with every open question either closed or listed as an assumption. Nothing is built against an unsigned design, however small the piece.

## Stage 3: Build

Entry: signed design document.
Work: development in two-week sprints. Each sprint ends with a demo to the client's project manager and a written note of what changed.
Exit: all agreed scope built, unit and integration tests passing, no open D1 or D2 defects, release notes written, and a UAT environment the client's testers can reach. The delivery lead confirms exit in the engagement log.

## Stage 4: UAT and sign-off

User acceptance testing normally runs for ten working days. The client's named testers run the agreed test scripts and log what they find. UAT closes when every script has been run and no D1 or D2 defect is open. The client sponsor then signs the UAT certificate. Go-live cannot be scheduled without it, and a verbal approval on a call does not replace it. Scripts added after UAT starts are a change request.

## Defect severity during UAT

Defect severity is separate from incident priority and uses its own labels. D1: blocks a core workflow and has no workaround. D2: breaks a function but a workaround exists. D3: cosmetic, or a minor deviation from the design. D4: a suggestion. D1 and D2 must be fixed before sign-off. D3 can move to the hypercare backlog with the client's written agreement. Do not call a UAT defect P1 or P2: those labels are for live incidents.

## Stage 5: Hypercare

Hypercare starts at go-live and runs for 30 calendar days for a new system, or 14 for a change to a live one. The build team stays assigned, checks the system by 10:30 every business day, and takes P1 and P2 incidents before any other work. Exit: no open P1 or P2 for 10 consecutive days, the runbook accepted by the support team, and the sponsor's confirmation in writing. If exit is not met on day 30, hypercare extends in 7 day steps and the Delivery Director is told.

## Stage 6: Handover to the client's team

Entry: hypercare exit met.
Work: two knowledge-transfer sessions of two hours each, a runbook, an architecture note and a list of known issues, all stored in the shared engagement folder.
Exit: the client's support team has run a live scenario from the runbook, our access to their systems is withdrawn within one working day, and the sponsor signs the handover note. The engagement is then closed in the log.

## Incident priority levels

P1 is a production outage or data loss that stops the client's operations. P2 is a major function that is degraded but has a workaround. P3 is a minor fault with little business impact. P4 is a request or a question. If you are unsure between two levels, choose the higher one and correct it later. The engineer on the ticket sets the level first, and the account owner confirms it with the client. Priority describes a live incident; UAT defects use D1 to D4.

## Response and resolution targets

| Priority | Acknowledge | Update the client | Mitigate within |
|---|---|---|---|
| P1 | 15 minutes, any hour | every 30 minutes | 4 hours |
| P2 | 1 hour in business hours | every 4 hours | 1 business day |
| P3 | next business day | every 2 business days | 5 business days |
| P4 | 3 business days | on completion | by agreement |
Business hours are 9:30 to 18:30 IST, Monday to Friday. Outside them only P1 is worked; a P2 raised after 18:30 is acknowledged at 9:30 on the next business day unless the contract buys extended cover.

## How the clock runs

Targets run from the time the ticket is created, not from the time someone first sees it. The clock pauses only while we wait for the client, and only if the pause is recorded on the ticket with the client's written agreement. Saturdays, Sundays and Karnataka public holidays count as outside business hours, so only P1 is worked, but a P1 is worked on every day of the year. If a client's contract states different targets, use the contract's and tell the delivery lead.

## When a P1 breaches its SLA

A priority-one (P1) ticket breaches its SLA if it is not acknowledged within 15 minutes or not mitigated within four hours. On a breach, do three things in order. First, page the delivery lead on call. Second, open an incident bridge and invite the account owner. Third, make sure the client hears from us first, with a call from the account owner inside 15 minutes of the bridge opening. If the delivery lead has not acknowledged the page within 10 minutes, page the Delivery Director directly. Note the time of each step in the incident record.

## On-call rotation

On-call rotates weekly, handing over at 10:00 IST every Monday. Each week has three people: a primary engineer who carries the pager and acknowledges pages, a secondary engineer who takes over if the primary has not acknowledged within 10 minutes, and a delivery lead on call who makes the decisions. Swaps are agreed in the rota channel at least 48 hours ahead and recorded. Whoever holds the pager stays responsible for an open incident until the next person acknowledges the handover.

## Escalation ladder

Move up the ladder at the times shown, counted from the page. Do not skip a rung unless it cannot be reached.
| Rung | Role | Engaged | Responds within |
|---|---|---|---|
| 1 | Primary on-call engineer | at the page | 15 minutes |
| 2 | Secondary engineer | primary silent | 10 minutes |
| 3 | Delivery lead on call | any P1 | 10 minutes |
| 4 | Delivery Director | lead silent, or P1 open 2 hours | 15 minutes |
| 5 | Managing Director | data loss, or a client executive asks | 30 minutes |

## Incident bridge and roles

The delivery lead opens the bridge and acts as incident commander. Four roles are named at the start: commander (decides), technical lead (diagnoses and fixes), scribe (keeps the timeline in the incident record) and client liaison (the account owner, who speaks to the client). One person does not hold two roles on a P1. The bridge stays open until the incident is mitigated, and the commander announces when it closes.

## Weekly status

A written status goes to the client every Thursday by 17:00 IST. It covers progress against plan, work for the coming week, open risks and the decisions we need. The weekly call is 30 minutes: the client's project manager, our delivery lead and the engineer leading the build attend. It produces the status note and an updated action log with an owner and a date against every item. Missed actions are carried forward in red, not dropped.

## Monthly steering committee

The steering committee meets in the first week of each month for 60 minutes. The client sponsor and two senior client stakeholders attend, with our account owner and delivery lead; the Delivery Director joins for engagements marked strategic. The agenda is fixed: delivery against plan, risks and decisions, change requests, and the next month. It produces minutes within two working days, a decision log and a refreshed risk register.

## Production change control

Every production change needs four things: a ticket, a review by a second engineer who did not write the change, a tested rollback plan, and an approval. The delivery lead approves a standard change. The Delivery Director approves a high-risk change, meaning a data migration, a model swap or a schema change. Each change is recorded with its time, approver and outcome. The rollback rule exists because a change once went out with a rollback plan that had never been run, and it did not work.

## Production changes and freezes

No production changes are made on Fridays after 3pm IST, during Deepavali week, in the week around Makara Sankranti, from 25 March to 5 April (the financial year end), or inside a client's declared freeze window. The only exception is an emergency fix for a P1, which needs the delivery lead's approval in writing. All other changes need a peer review and a rollback plan before they are deployed. Client freeze windows are recorded on the client record, so check it before you plan a release.

## Change requests

Any extra work outside the agreed scope, meaning anything beyond the signed statement of work, is a change request. The engineer who notices it logs it within one working day, in the client's own words. The delivery lead assesses effort and impact within three working days. The account owner prices it using the Pricing and Proposal Playbook and takes it to the client sponsor for written approval. No out-of-scope work starts before that approval. A request the client declines goes to the backlog, not into the build.

## Spotting scope creep

Scope creep is extra work that is not in the agreed scope, and it rarely arrives as a formal request. It arrives as "can it also..." on a call. Treat any of these as a change request: a new data source, a new group of users, an extra language, an integration the design does not name, or a change to an agreed success measure. Write it in the call notes the same day. In the call, say that it is logged and what happens next; do not agree to it or refuse it on the spot.

## Client systems and data under NDA

We reach a client's systems only through named accounts issued by the client's own administrator. Each account is entered in the access register with its owner, scope and end date. Client data stays inside the client's environment unless the contract says otherwise, and is never copied to personal storage or sent in a messaging app. Personal data is handled under the Digital Personal Data Protection Act, 2023 and the client's NDA. A suspected exposure is reported to the delivery lead within one hour.

## Working hours and overlap windows

Standard hours are 9:30 to 18:30 IST, Monday to Friday. For US East Coast clients the overlap window is 19:30 to 22:30 IST. For UK and Central European clients it is 12:30 to 17:30 IST. Overlap shifts are planned in the sprint, not improvised, and an engineer who works one takes the equivalent time off the next working day. Nobody works more than three overlap shifts a week without the delivery lead's approval.

## After an incident

Within three business days of any P1 or P2, the owner writes a blameless review covering what happened, how it was detected, what fixed it and what will change. The review meeting is held by day five, and actions have owners and dates by day seven. Reviews are shared with the whole team and filed with the client record. Actions are tracked to completion in the weekly meeting. The review is about the system and the process, not about the person who was on call.

## Handover between engineers

When an engineer hands over a live engagement, they write a handover note covering the architecture, access, known issues and open tickets, and walk the next engineer through it in person. A handover is not complete until the receiving engineer has been given access to every client system by the client's administrator, under their own named account. Keep the note in the shared engagement folder, not in a personal file.
