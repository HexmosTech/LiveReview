// Help text for every auto-approve / risk-policy control. Written in
// Simplified Technical English (ASD-STE100 style): short sentences, one idea
// per sentence, plain common words, active voice, and a concrete example
// wherever it helps. These settings are new, so most users will not guess
// their meaning from the label alone.

export const HELP = {
  // ---- Static Rules ----
  staticRules:
    'A static rule is a fixed check. It runs the same way on every pull request. ' +
    'Example: block the merge when a critical security finding is present. ' +
    "Static rules always run, even in 'Always approve' mode. " +
    'This stops a low-risk repository setting from hiding a serious problem.',

  // ---- Eligibility ----
  minAuthorLevel:
    'Each user has a review level. Level 1 is the lowest. ' +
    'Set the lowest level a pull request author must have to get an auto-approval. ' +
    'Example: set this to 3. Only authors at level 3 or higher can skip human review. ' +
    'An author below this level always needs a human reviewer, unless you add them to the whitelist.',

  forceApproveMinLevel:
    'This sets the lowest review level a user must have to force-approve a blocked review. ' +
    "A force-approve overrides the policy's block. The user must give a reason. " +
    'The system saves the reason in the audit log. ' +
    'Example: set this to 5. Only senior reviewers can then override a block.',

  // ---- General ----
  riskThresholdPercentile:
    'This sets how strict the risk check is. ' +
    "The system scores every review from 0 (safe) to 100 (risky). " +
    "It compares each new score to your team's recent scores. " +
    'Example: P20 approves only the safest 20 out of 100 reviews. ' +
    'A lower number is stricter. A higher number allows more auto-approvals.',

  requireLLMConfidence:
    'The AI reviewer also labels the type of change, for example "refactor-only" or "security-sensitive". ' +
    'It gives a confidence score from 0 to 10 for this label. ' +
    'Set the lowest confidence score the policy will accept. ' +
    'Example: set this to 8. The system blocks the auto-approval when the AI confidence is below 8.',

  landingDelaySeconds:
    'This is a wait time after an auto-approval, before the system posts it. ' +
    'During this time, a human can still step in and stop the merge. ' +
    'Example: set this to 600 for a 10-minute wait. ' +
    'Set it to 0 to post the approval right away.',

  dailyApproveCap:
    'This sets the highest number of auto-approvals allowed per day. ' +
    'Example: set this to 20. The system stops auto-approving after 20 reviews that day. ' +
    'Leave this field blank for no limit.',

  restrictOutcomeReporting:
    'This switch controls who can report a revert or an incident from the review page. ' +
    'When it is on, only owners and admins can report one. ' +
    'When it is off, any team member can report one. ' +
    'The webhook always needs its own secret key. This switch does not change that.',

  // ---- Risk Model tab ----
  activeModel:
    'This is the risk model that scores reviews right now. ' +
    'A "seed" model uses general, common-sense rules only. It has not learned from your data yet. ' +
    "A \"trained\" model has learned from the outcomes your team reported.",

  categoryOverrideIntro:
    'Each slider changes how strongly one type of signal affects the risk score. ' +
    'A value of 1× keeps the normal weight. A value of 2× doubles its effect. ' +
    'A change here takes effect right away. It does not need a retrain.',

  categoryBlastRadius:
    'Blast radius measures how much other code a change can affect. ' +
    'A large blast radius means higher risk. ' +
    'Raise this slider to make the risk score react more strongly to large, far-reaching changes.',

  categorySecurityFindings:
    'This counts findings tagged as security issues. ' +
    'Raise this slider to make the risk score react more strongly to security findings.',

  categoryCriticalFindings:
    'This counts findings marked "critical". ' +
    'Raise this slider to make the risk score react more strongly to critical findings.',

  categoryDiffSize:
    'This measures how many files and lines changed. ' +
    'Raise this slider to make the risk score react more strongly to large changes.',

  retrainNow:
    'This retrains the risk model right now, using every outcome your team has reported so far. ' +
    'The system also retrains once a day on its own. ' +
    'Use this button after you report new outcomes, or change a weight, and want the model to update sooner.',

  simulate:
    "This tests the current draft policy against your team's recent completed reviews. " +
    'It shows what the policy would have done for each review. ' +
    'No approval is sent. No setting is saved. ' +
    'Use this to check a policy before you save it.',

  // ---- Production Feedback tab ----
  outcomeWebhook:
    'Your CI/CD system or incident tool can send a message to this address. ' +
    'It can report a revert, an incident, or confirm that a change was safe. ' +
    'Example: a pipeline step runs "git revert", then posts to this address with outcome_type set to "revert".',

  outcomeCounter:
    'This shows how many outcomes your team reported in the last 30 days. ' +
    'More reports help the risk model learn faster. ' +
    'Even a few reports each month are enough to start.',

  // ---- Repository override ----
  repoMode:
    'Choose how this repository uses the policy. ' +
    "'Inherit' uses the organization's default policy. " +
    "'Custom' lets you set different rules for only this repository. " +
    "'Always approve' skips the risk check, but static rules still run. " +
    "'Never approve' blocks every pull request from auto-approval.",

  repoSharedModel:
    'The trained risk model is shared by the whole organization. ' +
    'Only the cutoff score, and the other settings below, can be different for this repository.',

  // ---- Review detail funnel (Auto Approve tab) ----
  stageEligibility:
    "This checks if the pull request author's review level is high enough to skip human review, " +
    'or if the author is on the whitelist.',

  stageStatic:
    "This runs your team's fixed checks, for example a check for leaked secrets in the diff. " +
    'A static rule can block the approval even when every other check passes.',

  stageRisk:
    "This is the trained model's risk score for this change. " +
    "The system compares it to your team's threshold. " +
    'A higher score means higher risk.',

  stageLLM:
    'This is the AI\'s label for the type of change, for example "logic-change" or "security-sensitive", ' +
    'with a confidence score.',

  stageFinal:
    "This is the policy's final answer: approve or block. " +
    'It is the result of every stage above, combined.',

  forceApproveSection:
    "Force approve overrides a blocked review's decision. " +
    'Only a user at or above the required review level, or an owner/admin, can do this. ' +
    'You must give a reason. ' +
    'The system saves this action in the audit log, and posts a warning comment on the pull request.',

  markOutcomeSection:
    'Use this to tell the risk model what happened after this change went live. ' +
    '"Revert" means the team rolled the change back. ' +
    '"Incident" means the change caused a production problem. ' +
    '"Confirmed safe" means the change caused no problems. ' +
    'Your report helps the model learn.',
};
