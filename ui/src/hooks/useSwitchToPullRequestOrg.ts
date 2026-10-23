import { useAppDispatch, useAppSelector } from '../store/configureStore';
import { loadUserOrganizations, switchOrganization } from '../store/Organizations/reducer';
import { getPullRequestOrg } from '../api/pullRequests';

export const PR_NOT_FOUND_MESSAGE =
  "This pull request wasn't found, or you don't have access to its organization.";

// Thrown when the user's org has the PR's GitHub account available but not connected yet (the switch to that org
// has already happened): connecting it brings the PR in.
export class ConnectRequiredError extends Error {
  constructor(readonly connectorId: number, readonly accountLogin: string, readonly canConnect: boolean) {
    super(`${accountLogin} isn't connected to this LiveReview account yet. Connect it under Git Providers, GitHub App.`);
    this.name = 'ConnectRequiredError';
  }
}

// PR comment links don't carry an org, so look up the PR's org and switch to it.
// Resolves to the pull request id to use there: orgs sharing a GitHub App installation each
// have their own copy of the PR, and the link may name another org's copy.
// Throws the API error (status 404 when missing or not a member), or ConnectRequiredError.
export const useSwitchToPullRequestOrg = () => {
  const dispatch = useAppDispatch();
  const { currentOrgId, userOrganizations } = useAppSelector((state) => state.Organizations);

  return async (pullRequestId: number): Promise<number> => {
    const { org_id: orgId, pull_request_id: resolvedId, connect } = await getPullRequestOrg(pullRequestId);
    if (currentOrgId !== orgId) {
      if (!userOrganizations.some((o) => o.id === orgId)) {
        await dispatch(loadUserOrganizations()).unwrap();
      }
      await dispatch(switchOrganization({ orgId })).unwrap();
    }
    if (connect) {
      throw new ConnectRequiredError(connect.connector_id, connect.account_login, connect.can_connect);
    }
    if (resolvedId === undefined) {
      throw new Error('Failed to look up the pull request.');
    }
    return resolvedId;
  };
};
