import { useAppDispatch, useAppSelector } from '../store/configureStore';
import { loadUserOrganizations, switchOrganization } from '../store/Organizations/reducer';
import { getPullRequestOrg } from '../api/pullRequests';

export const PR_NOT_FOUND_MESSAGE =
  "This pull request wasn't found, or you don't have access to its organization.";

// PR comment links don't carry an org, so look up the PR's org and switch to it.
// Throws the API error (status 404 when missing or not a member).
export const useSwitchToPullRequestOrg = () => {
  const dispatch = useAppDispatch();
  const { currentOrgId, userOrganizations } = useAppSelector((state) => state.Organizations);

  return async (pullRequestId: number) => {
    const { org_id: orgId } = await getPullRequestOrg(pullRequestId);
    if (currentOrgId === orgId) return;
    if (!userOrganizations.some((o) => o.id === orgId)) {
      await dispatch(loadUserOrganizations()).unwrap();
    }
    await dispatch(switchOrganization({ orgId })).unwrap();
  };
};
