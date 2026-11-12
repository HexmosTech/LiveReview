import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { UserManagement } from './UserManagement';
import { fetchOrgUsers, deactivateOrgUser } from '../../api/users';

const mockMember = {
    id: 42,
    first_name: 'Jane',
    last_name: 'Doe',
    email: 'jane@example.com',
    is_active: true,
};

jest.mock('../../hooks/useOrgContext', () => ({
    useOrgContext: () => ({
        currentOrg: { id: 1, name: 'Acme' },
        isSuperAdmin: false,
        canManageCurrentOrg: true,
        isFreePlan: false,
    }),
}));

jest.mock('../../api/users', () => ({
    fetchOrgUsers: jest.fn(),
    deactivateOrgUser: jest.fn(),
}));

jest.mock('../../utils/notify', () => ({
    notify: { success: jest.fn(), error: jest.fn() },
}));

jest.mock('../../utils/deploymentMode', () => ({
    isCloudMode: () => false,
}));

jest.mock('../../hooks/useLicenseTier', () => ({
    useLicenseTier: () => 'team',
    useHasLicenseFor: () => true,
    COMMUNITY_TIER_LIMITS: { MAX_USERS: 5 },
}));

jest.mock('react-redux', () => ({
    useSelector: () => ({ status: 'active', loading: false }),
}));

jest.mock('react-router-dom', () => {
    const ReactModule = require('react');
    return {
        useNavigate: () => jest.fn(),
        Link: ({ children }: { children: unknown }) =>
            ReactModule.createElement(ReactModule.Fragment, null, children),
    };
});

jest.mock('../License/LicenseUpgradeDialog', () => () => null);

jest.mock('./UserList', () => {
    const ReactModule = require('react');
    return {
        UserList: ({ onDeactivateUser }: { onDeactivateUser?: (user: typeof mockMember) => void }) =>
            ReactModule.createElement(
                'button',
                { onClick: () => onDeactivateUser?.(mockMember) },
                'Deactivate member'
            ),
    };
});

describe('UserManagement deactivation confirmation', () => {
    beforeEach(() => {
        (fetchOrgUsers as jest.Mock).mockResolvedValue({
            members: [mockMember],
            total: 1,
            total_pages: 1,
        });
        (deactivateOrgUser as jest.Mock).mockResolvedValue({});
    });

    it('shows the in-app confirmation modal instead of a native browser dialog', async () => {
        const confirmSpy = jest.spyOn(window, 'confirm').mockImplementation(() => true);
        const alertSpy = jest.spyOn(window, 'alert').mockImplementation(() => undefined);

        render(<UserManagement />);
        fireEvent.click(await screen.findByRole('button', { name: 'Deactivate member' }));

        expect(await screen.findByText('Deactivate user?')).toBeInTheDocument();
        expect(screen.getByText('Are you sure you want to deactivate Jane Doe?')).toBeInTheDocument();
        expect(confirmSpy).not.toHaveBeenCalled();
        expect(alertSpy).not.toHaveBeenCalled();
        expect(deactivateOrgUser).not.toHaveBeenCalled();

        fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
        await waitFor(() => expect(screen.queryByText('Deactivate user?')).not.toBeInTheDocument());
        expect(deactivateOrgUser).not.toHaveBeenCalled();

        confirmSpy.mockRestore();
        alertSpy.mockRestore();
    });

    it('deactivates the user only after confirming in the modal', async () => {
        render(<UserManagement />);
        fireEvent.click(await screen.findByRole('button', { name: 'Deactivate member' }));
        fireEvent.click(await screen.findByRole('button', { name: 'Deactivate' }));

        await waitFor(() => expect(deactivateOrgUser).toHaveBeenCalledWith('1', '42'));
    });
});
