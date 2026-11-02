# Demo documents

Three short documents for a fictional company, written so you can see Keystone work before you have documents of your own. The company, its people, its prices and its policies are invented. Nothing in the code depends on them. Delete them, or never upload them, when you bring your own.

| File | What it covers | Passages | Who reads it in the sample setup |
|---|---|---|---|
| `pricing-and-proposal-playbook.md` | Pricing, discounts, proposals | 26 | Sales & Marketing |
| `leave-and-benefits-policy.md` | Leave, holidays, benefits | 24 | HR |
| `delivery-and-escalation-sop.md` | Delivery stages, incident response, deploy rules | 26 | Delivery |

"Passages" is how many pieces each file is split into when Keystone ingests it. The sample teams are the three in `.env.example` (`KEYSTONE_TEAMS`). If you changed the team list, tick whichever teams you like when you upload.

In the screenshots in the main README these are uploaded with the titles "Sales and Marketing Playbook", "HR Policies" and "Delivery Handbook". The title is whatever you type at upload, so the file name does not have to match.

## Questions to try

Sign in as an employee of the Delivery team and ask:

| Question | What you should see |
|---|---|
| What do I do when a priority-one ticket breaches its SLA? | An answer from the Delivery document, with numbered citations |
| When am I not allowed to deploy to production? | An answer from the Delivery document |
| What discount can I offer a client who commits to a twelve month retainer? | Refused. The best match is in the playbook, which Delivery cannot read, so Keystone names Sales & Marketing and shows an empty slot |
| How many days of annual leave do I get? | Refused, naming HR, until HR Policies is made readable to Delivery |
| What is the office Wi-Fi password? | No match. No document answers it |

Then sign in as the admin, open the playbook's Access page, give Delivery access, and ask the retainer question again. This time it is answered, with citations.

These results come from the default settings and the default embedding model (`voyage-4-lite`). A different embedding model scores differently, so a borderline question can land on the other side of a threshold. See "Tuning the thresholds" in the main README.
