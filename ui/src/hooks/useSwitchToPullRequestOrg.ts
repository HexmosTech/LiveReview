import { useAppDispatch, useAppSelector } from '../store/configureStore';
import { loadUserOrganizations, switchOrganization } from '../store/Organizations/reducer';
import { getPullRequestOrg } from '../api/pullRequests';

export const PR_NOT_FOUND_MESSAGE =
  "This pull request wasn't found, or you don't have access to its organization.";

// PR comment links don't carry an org, so look up the PR's org and switch to it.
// Resolves to the pull request id to use there: orgs sharing a GitHub App installation each
// have their own copy of the PR, and the link may name another org's copy.
// Throws the API error (status 404 when missing or not a member).
export const useSwitchToPullRequestOrg = () => {
  const dispatch = useAppDispatch();
  const { currentOrgId, userOrganizations } = useAppSelector((state) => state.Organizations);

  return async (pullRequestId: number): Promise<number> => {
    const { org_id: orgId, pull_request_id: resolvedId } = await getPullRequestOrg(pullRequestId);
    if (currentOrgId !== orgId) {
      if (!userOrganizations.some((o) => o.id === orgId)) {
        await dispatch(loadUserOrganizations()).unwrap();
      }
      await dispatch(switchOrganization({ orgId })).unwrap();
    }
    return resolvedId;
  };
};
