LiveReview supports various Git-based code hosting services. You can add a new Git provider via `Git Providers -> Select Git Provider`.

<img width="957" height="440" alt="image" src="https://github.com/user-attachments/assets/9b7a20d7-9e00-45f3-a075-c488a1f34ac5" />

Regardless of the provider, we recommend the following method of integration:

1. Create a custom user with a name starting with `LiveReviewBot`. Example: `LiveReviewBot1850`.
2. Grant this user the relevant project access.
3. Generate an API key for the user.
4. Provide LiveReview with the API key and user details.
5. Trigger AI code reviews on MRs.

Steps (1), (2), and (3) vary slightly by platform, so we will provide exact instructions or resources for each.

## But why don’t you automatically integrate with the platform?

We experimented with automatic integration but found too many pitfalls, such as:

1. Automatic user creation APIs are highly limited, and many necessary actions depend on subtle or unclear API details of the respective platforms. Automatic user management via APIs across platforms is not reliable.
2. In MR comments, the name shown will usually be that of a human user with auto integration, which is misleading.
3. It is harder to audit and control access via automatic integration in a unified way across platforms.
4. Automatic integration limits the level of MR interaction possible.

Overall, we **strongly recommend** following the steps outlined below to create Git integration. This is a *one-time action* which, once configured, requires minimal to no maintenance. The productivity benefits of automatic AI code review will make this setup effort worthwhile.

## Git Integration Guide by Platform

- [Self-hosted GitLab](Self‐Hosted-GitLab)
- [Gitlab](Gitlab)
- [Github](Github)
- [BitBucket](BitBucket)
