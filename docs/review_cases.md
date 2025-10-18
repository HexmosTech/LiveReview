These are all the cases which my code is supposed to handle on all 3 platforms


The following things must work across all platforms - github, gitlab, bitbucket, any other new ones uniformly:

Trigger Review 1

Expectation: fetch the diff, added lines, removed lines, provide contextually helpful comments

Select a diff line and ask clarification question without bot mention

Expectation: No response

Select a diff line and ask clarification question with bot mention

Expectation: Contextual response

Reply to a bot comment from (1) without bot mention

Expectation: Contextual response

Reply to a bot comment from (1) with bot mention

Expectation: Contextual response

Agree/Disagree to a bot comment from (1) with or without bot mention

Learning extraction

Comment about learning

Database update

Trigger Review 2

Doesn't duplicate existing comments (context aware)

Uses learnings from (6) to further assess

Looks at new or deleted code from the previous review (again, context aware)


The idea of "context awareness" means:

Timeline of commits/changes
Timeline of comments/responses
Response tree representation, time/sequence awareness
View of "code evolution" via diffs in an MR
Applicable "Learnings" from the past MRs or present MRs