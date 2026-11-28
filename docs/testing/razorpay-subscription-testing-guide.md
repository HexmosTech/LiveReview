# Razorpay Subscription & Free Trial Testing Guide

This guide provides step-by-step instructions and test data to verify the LiveReview subscription lifecycle, Razorpay test mode checkouts, 7-day free trials, mid-trial upgrades, and paid subscription mid-cycle upgrades (prorated).

---

## 1. Test Credentials & Card Details

Use these Razorpay test payment credentials in test mode (`RAZORPAY_MODE=test`):

### A. Credit / Debit Cards (Auto-Recurring Mandate)

| Field | Value |
|---|---|
| **Card Number (Visa)** | `4111 1111 1111 1111` |
| **Card Number (Mastercard)** | `5123 4567 8901 2345` |
| **Card Number (RuPay)** | `5081 5999 9999 9999` |
| **Expiry Date** | Any future date (e.g., `12/30`) |
| **CVV** | Any 3 digits (e.g., `123`) |
| **Cardholder Name** | Any name (e.g., `LiveReview Tester`) |
| **3D Secure OTP** | `1234` or `123456` (or click the green **Success** button on the bank simulator) |

### B. UPI Autopay / eMandate Test

| Field | Value |
|---|---|
| **VPA / UPI ID** | `success@razorpay` |
| **Action** | Approve mandate on the simulated popup / click **Success** |

### C. Netbanking eMandate Test

| Field | Value |
|---|---|
| **Bank** | Select **HDFC Bank** or **ICICI Bank** |
| **Action** | Click **Authorize / Success** on the simulated bank portal |

---

## 2. Test Scenarios Checklist

### Scenario 1: First Purchase 7-Day Free Trial (From Free 30k Plan)

**Goal**: Verify a user on the Free tier can start a 7-day free trial on any selected paid tier.

1. Sign in to LiveReview and navigate to **Settings → Subscriptions** (`/settings#subscriptions`).
2. Ensure the organization is on the **Free 30k BYOK** tier.
3. **Verify UI**:
   - Every upgrade card (100k, 200k, 400k, 800k, 1.6M, 3.2M LOC) displays the badge:  
     `FREE 7-DAY TRIAL INCLUDED`
4. Click **Upgrade** on a plan (e.g., **Team 32 USD / 100k LOC** or **Team 64 USD / 200k LOC**).
5. The **Confirm New Paid Plan** modal appears:
   - Notice text: Explains that moving to this paid plan includes a 7-day free trial.
   - Shows included LOC and selected billing currency (INR or USD).
6. Click **Confirm and Pay Now**.
7. The Razorpay checkout dialog opens:
   - Shows authorization / token setup amount (e.g., ₹5 or $0 setup auth).
8. Enter test card `4111 1111 1111 1111`, expiry `12/30`, CVV `123`.
9. Click **Pay Now** and enter OTP `1234` (or click **Success**).
10. **Verify Results**:
    - Notification indicates subscription setup was successful.
    - Banner shows: **Active Free Trial** (ends in 7 days).
    - Plan limit immediately updates to the selected plan's LOC (e.g., 100,000 LOC or 200,000 LOC).
    - Reviews can be performed up to the granted LOC without restriction.

---

### Scenario 2: Mid-Trial Upgrade (Early Upgrade to Higher Tier)

**Goal**: Verify that upgrading while on an active trial terminates the trial immediately, charges the full amount upfront, and resets the 30-day billing cycle to today.

1. Ensure the organization currently has an active free trial (e.g., on 100k LOC from Scenario 1).
2. Go to **Settings → Subscriptions** (`/settings#subscriptions`).
3. Under *Upgrade Plan to Get More Usage*, click **Upgrade** on a higher plan (e.g., **Team 64 USD / 200k LOC** or **Team 128 USD / 400k LOC**).
4. The **Confirm Plan Upgrade (Ends Free Trial)** modal opens:
   - Heading: `Confirm Plan Upgrade (Ends Free Trial)`
   - Explanatory text:  
     *"Upgrading now ends your free trial. Your paid billing cycle begins immediately with your new LOC limit, and your scheduled trial charge is cancelled."*
   - Payment section: Shows `Payment required now` with full plan price (e.g., US$64.00 or ₹5,300).
   - Button label: **`Pay Full Amount and Upgrade`**.
5. Click **Pay Full Amount and Upgrade**.
6. The Razorpay checkout popup opens for the full charge amount.
7. Enter test card details (`4111 1111 1111 1111`), click Pay, and complete authorization.
8. **Verify Results**:
   - The **Active Free Trial** banner disappears.
   - Current plan updates to the target tier (e.g., Team 64 USD / 200k LOC).
   - Monthly LOC quota updates immediately to 200,000 LOC.
   - Next billing renewal date is set to **exactly 30 days from today**.
   - No trial charge is scheduled at the original Day 7 mark.

---

### Scenario 3: Paid Subscription Mid-Cycle Upgrade (Prorated Upgrade)

**Goal**: Verify that upgrading mid-cycle while on an active paid subscription calculates a prorated charge based on the remaining cycle fraction, grants prorated additional LOC, and preserves the existing billing renewal date.

**Key Difference: Active Paid vs. Active Trial**:
When an organization is on an active paid plan (the initial trial has completed, `trial_ends_at` is `NULL` or expired, and provider status is `active`), upgrading mid-cycle does **not** charge full price or reset the billing date. Instead, it calculates proration based on the remaining cycle fraction.

| Attribute | Scenario 2: Mid-Trial Upgrade | Scenario 3: Paid Mid-Cycle Upgrade (Prorated) |
|---|---|---|
| **Current Org State** | Active 7-day free trial (`trial_ends_at` in the future, provider `authenticated`) | Active paid subscription (`trial_ends_at` is `NULL` or expired, provider `active`) |
| **Modal Heading** | `Confirm Plan Upgrade (Ends Free Trial)` | `Confirm Plan Upgrade` / `Confirm Upgrade and Prorated Charge` |
| **Explanatory Text** | *"Upgrading now ends your free trial. Your paid billing cycle begins immediately with your new LOC limit..."* | *"This charge applies now for the remaining current cycle. Upgrade grant is finalized after deterministic payment..."* |
| **Immediate Charge** | **Full plan price** upfront (e.g., US$64.00) | **Prorated amount** based on remaining cycle fraction: `Target Price × (Remaining Seconds / Total Cycle Seconds)` |
| **Billing Cycle Renewal** | **Resets to today**: next renewal is 30 days from today | **Preserved**: keeps existing cycle renewal date (`Current cycle ends: <date>`) |
| **Action Button** | **`Pay Full Amount and Upgrade`** | **`Confirm and Pay Now`** |

**Step-by-Step Test Procedure**:
1. Ensure the organization is on an active paid subscription (e.g., Team 100k LOC, billing cycle active with e.g. 20 days remaining out of 30).
2. Go to **Settings → Subscriptions** (`/settings#subscriptions`).
3. Under *Upgrade Plan to Get More Usage*, click **Upgrade** on a higher plan (e.g., **Team 64 USD / 200k LOC**).
4. The **Confirm Plan Upgrade** (or `Confirm Upgrade and Prorated Charge`) modal opens:
   - **Modal Heading**: `Confirm Plan Upgrade` (or `Confirm Upgrade and Prorated Charge`).
   - **Plan Details**: Shows `Current plan: Team 100k LOC` and `Target plan: Team 200k LOC`.
   - **Cycle End**: Shows `Current cycle ends: <date>` (verifying the current billing cycle renewal date is **preserved**, not reset).
   - **Payment Section**: Shows `Immediate one-time payment` with the prorated amount (e.g., `Charge now: US$21.33 (remaining cycle 66.67%)`).
   - **Formula Display**: Shows `Formula: US$32.00 × 66.67%`.
   - **Immediate LOC Grant**: Shows prorated additional LOC grant for the remainder of the cycle.
   - **Next Billing Cycle**: Displays recurring amount for next cycle (`US$64.00/month`) and full monthly limit (`200,000 LOC`).
   - **Button label**: **`Confirm and Pay Now`** (unlike Mid-Trial Upgrade's `Pay Full Amount and Upgrade`).
5. Click **Confirm and Pay Now**.
6. The Razorpay checkout dialog opens for the exact prorated charge amount (not the full monthly price).
7. Enter test card details (`4111 1111 1111 1111`), click Pay, and complete authorization.
8. **Verify Results**:
   - Current plan updates to the target tier (e.g., Team 64 USD / 200k LOC).
   - LOC quota increases immediately with the prorated additional LOC grant.
   - Next billing renewal date (`billing.billing_period_end`) **remains identical** to the existing cycle renewal date.
   - When the preserved renewal date arrives, the recurring charge will be the full monthly rate of the new plan.

---

### Scenario 4: Depleted Trial Quota Upgrade

**Goal**: Verify behavior when an organization consumes all trial LOC before Day 7.

1. Set up an org on a 100k LOC trial.
2. Run reviews until the LOC quota reaches 100,000 LOC (or usage exceeds quota).
3. **Verify Block State**:
   - Review operations display quota-exceeded / trial-readonly block banner.
4. Go to **Settings → Subscriptions** and click **Upgrade** on 200k LOC.
5. Complete payment for the full plan amount.
6. **Verify Results**:
   - Block state clears immediately (`loc_blocked = false`, `trial_readonly = false`).
   - Organization has new 200,000 LOC limit.
   - Reviews can resume immediately.

---

### Scenario 5: One-Time Trial Enforcement (No Repeated Trials)

**Goal**: Verify that an organization or email that has previously consumed a trial cannot receive a second trial.

1. In an organization that already used or upgraded from a trial:
2. Downgrade to Free 30k (or switch org with the same owner email).
3. When viewing the upgrade cards:
   - Cards must **not** display `Free 7-Day Trial Included`.
   - Cards display `Trial already used` or standard `Upgrade`.
4. Proceeding to checkout requires immediate recurring payment without trial period.

---

### Scenario 6: Multi-Currency Switching (INR vs. USD)

**Goal**: Verify currency conversion and plan pricing resolution.

1. Open **Settings → Subscriptions**.
2. Switch the currency dropdown between **INR** and **USD**.
3. **Verify**:
   - USD displays prices formatted with `US$` (e.g., `US$32.00/month`, `US$64.00/month`).
   - INR displays prices formatted with `₹` (e.g., `₹2,650/month`, `₹5,300/month`).
   - Checkout popup opens in the selected currency.

---

### Scenario 7: Cancellation During Active Trial

**Goal**: Verify cancellation before Day 7 prevents any recurring charge.

1. On an active 7-day trial:
2. Open the **Subscription Control** tab.
3. Click **Cancel Subscription**.
4. Confirm cancellation.
5. **Verify**:
   - Status updates to cancelled / scheduled expiry.
   - Razorpay subscription is cancelled on the gateway.
   - No charge is processed when the 7-day window expires.

---

## 3. Quick Verification Commands

Check the subscription status and billing state via authenticated API:

```bash
# Get org billing status
curl -s -H "Authorization: Bearer <JWT_TOKEN>" \
  -H "X-Org-Context: <ORG_ID>" \
  https://<HOST>/api/v1/billing/status | jq .
```

Verify key response fields:
- `billing.current_plan_code`: active plan (e.g., `team_32usd`, `loc_200k`).
- `billing.trial_readonly`: `false` during active trial or paid subscription.
- `billing.trial_ends_at`: ISO timestamp 7 days ahead (null once upgraded to paid).
- `billing.billing_period_end`: 30 days from purchase/upgrade date for new/trial upgrades; preserved existing cycle end date for paid mid-cycle upgrades.

