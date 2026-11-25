import React from 'react';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import ThirdPartyToolsTab from './ThirdPartyToolsTab';
import apiClient from '../../api/apiClient';
import { useOrgContext } from '../../hooks/useOrgContext';

jest.mock('../../api/apiClient', () => ({
  __esModule: true,
  default: {
    get: jest.fn(),
    put: jest.fn(),
  },
}));

jest.mock('../../hooks/useOrgContext', () => ({
  useOrgContext: jest.fn(),
}));

const mockApiGet = apiClient.get as jest.Mock;
const mockUseOrgContext = useOrgContext as jest.Mock;

describe('ThirdPartyToolsTab - tool info popover & official GitHub link', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseOrgContext.mockReturnValue({
      currentOrg: { id: 7, name: 'Test Org', role: 'owner' },
      canManageCurrentOrg: true,
    });
    mockApiGet.mockImplementation((url: string) => {
      if (url === '/orgs/7/tools') {
        return Promise.resolve({
          tools: [
            {
              id: 1,
              name: 'trivy',
              description: 'All-in-one vulnerability and misconfiguration scanner for lockfiles, Dockerfiles, Kubernetes YAML, and Terraform.',
              lambda_arn: 'trivy-linter',
              multiplier: 4.0,
              use_case: 'Containers, IaC & SCA CVEs',
              category: 'Dependency Vulnerabilities',
              docs_url: 'https://github.com/aquasecurity/trivy',
              enabled: true,
            },
            {
              id: 2,
              name: 'unsafe-tool',
              description: 'Tool with invalid URL scheme should not render an anchor.',
              lambda_arn: 'unsafe-linter',
              multiplier: 1.0,
              use_case: 'Validation test',
              category: 'Code Quality',
              docs_url: 'javascript:alert(1)',
              enabled: false,
            },
          ],
        });
      }
      if (url === '/billing/usage/summary') {
        return Promise.resolve({
          tool_credits_used_month: 1000,
          tool_credits_limit_month: 50000,
        });
      }
      return Promise.resolve({});
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('opens info popover with what tool is about and official GitHub link (without credit/budget noise)', async () => {
    render(<ThirdPartyToolsTab />);

    const infoBtn = await screen.findByRole('button', { name: 'Inspect trivy details' });
    expect(screen.queryByText(/All-in-one vulnerability and misconfiguration scanner/)).not.toBeInTheDocument();

    fireEvent.click(infoBtn);

    const dialog = screen.getByRole('dialog', { name: 'trivy information' });
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText(/All-in-one vulnerability and misconfiguration scanner/)).toBeInTheDocument();
    expect(screen.queryByText(/solo runs in budget/i)).not.toBeInTheDocument();

    const link = screen.getByRole('link', { name: /Official GitHub repository/i });
    expect(link).toHaveAttribute('href', 'https://github.com/aquasecurity/trivy');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'trivy information' })).not.toBeInTheDocument();
  });

  it('shows info popover on hover and blocks non-http/https links', async () => {
    jest.useFakeTimers();
    render(<ThirdPartyToolsTab />);

    const infoBtn = await screen.findByRole('button', { name: 'Inspect unsafe-tool details' });
    fireEvent.mouseEnter(infoBtn);

    expect(await screen.findByRole('dialog', { name: 'unsafe-tool information' })).toBeInTheDocument();
    expect(screen.getByText('Tool with invalid URL scheme should not render an anchor.')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: /Official GitHub repository/i })).not.toBeInTheDocument();

    fireEvent.mouseLeave(infoBtn);
    act(() => {
      jest.advanceTimersByTime(200);
    });
    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: 'unsafe-tool information' })).not.toBeInTheDocument();
    });
  });
});
