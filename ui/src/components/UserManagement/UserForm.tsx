import React, { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import * as z from 'zod';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';
import { useOrgContext } from '../../hooks/useOrgContext';
import { createOrgUser, fetchOrgUser, updateOrgUser, Member, checkUserByEmail } from '../../api/users';
import { Button, Input, Select } from '../UIPrimitives';
import { useAppDispatch } from '../../store/configureStore';
import { loadUserOrganizations } from '../../store/Organizations/reducer';
import { UpgradePromptModal } from '../Subscriptions';
import { UserOnboardingDetails } from './UserOnboardingDetails';

const baseSchema = z.object({
  email: z.string().email({ message: 'Invalid email address' }),
  firstName: z.string().optional(),
  lastName: z.string().optional(),
  role: z.enum(['member', 'owner', 'super_admin']),
  password: z.string().optional(),
  password_confirmation: z.string().optional(),
});

type UserFormData = z.infer<typeof baseSchema>;

const UserForm: React.FC = () => {
  const navigate = useNavigate();
  const dispatch = useAppDispatch();
  const { userId } = useParams<{ userId: string }>();
  const { currentOrgId, currentOrg } = useOrgContext();
  const currentUserRole = currentOrg?.role;

  const isEditMode = !!userId;

  const [existsGlobally, setExistsGlobally] = useState(false);
  const [checkingEmail, setCheckingEmail] = useState(false);

  const userSchema = baseSchema.refine(
    (data) => {
      if (!isEditMode && !existsGlobally) {
        return data.firstName && data.firstName.length > 0;
      }
      return true;
    },
    {
      message: 'First name is required for new users',
      path: ['firstName'],
    }
  ).refine(
    (data) => {
      if (!isEditMode && !existsGlobally) {
        return data.lastName && data.lastName.length > 0;
      }
      return true;
    },
    {
      message: 'Last name is required for new users',
      path: ['lastName'],
    }
  ).refine(
    (data) => {
      if (!isEditMode && !existsGlobally) {
        return data.password && data.password.length >= 8;
      }
      return true;
    },
    {
      message: 'Password must be at least 8 characters',
      path: ['password'],
    }
  ).refine(
    (data) => {
      if (!isEditMode && !existsGlobally) {
        return data.password === data.password_confirmation;
      }
      return true;
    },
    {
      message: 'Passwords do not match',
      path: ['password_confirmation'],
    }
  );

  const [user, setUser] = useState<Member | null>(null);
  const [createdUser, setCreatedUser] = useState<Member | null>(null);
  const [loading, setLoading] = useState(false);
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
    reset,
    watch,
    setValue,
    trigger,
  } = useForm<UserFormData>({
    resolver: zodResolver(userSchema),
    defaultValues: {
      role: 'member',
    },
  });

  const emailValue = watch('email');

  const handleEmailCheck = async () => {
    if (!currentOrgId || isEditMode || !emailValue || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailValue)) {
      return;
    }

    setCheckingEmail(true);
    try {
      const result = await checkUserByEmail(currentOrgId.toString(), emailValue);
      setExistsGlobally(result.exists);
      if (result.exists) {
        setValue('firstName', result.first_name || '');
        setValue('lastName', result.last_name || '');
        // Clear password errors if any
        trigger();
      }
    } catch (error) {
      console.error('Failed to check email', error);
    } finally {
      setCheckingEmail(false);
    }
  };

  useEffect(() => {
    if (userId && currentOrgId) {
      setLoading(true);
      fetchOrgUser(currentOrgId.toString(), userId)
        .then(userData => {
          setUser(userData);
          reset({
            email: userData.email,
            firstName: userData.first_name || '',
            lastName: userData.last_name || '',
            role: userData.role as 'member' | 'owner' | 'super_admin' || 'member',
          });
        })
        .catch(err => {
          toast.error(`Failed to load user: ${err.message}`);
          navigate('/settings#users');
        })
        .finally(() => setLoading(false));
    }
  }, [userId, currentOrgId, reset, navigate]);

  const getRoleOptions = () => {
    if (currentUserRole === 'super_admin') {
      return [
        { value: 'member', label: 'Member' },
        { value: 'owner', label: 'Owner' },
        // { value: 'super_admin', label: 'Super Admin' },
      ];
    }
    if (currentUserRole === 'owner') {
      return [
        { value: 'member', label: 'Member' },
        { value: 'owner', label: 'Owner' },
      ];
    }
    return [{ value: 'member', label: 'Member' }];
  };

  const roleNameToId = (roleName: 'member' | 'owner' | 'super_admin'): number => {
    switch (roleName) {
      case 'super_admin':
        return 1;
      case 'owner':
        return 2;
      case 'member':
        return 3;
      default:
        return 3; // Default to member
    }
  };

  const onSubmit = async (data: UserFormData) => {
    if (!currentOrgId) {
      toast.error('No organization selected.');
      return;
    }

    try {
      if (isEditMode && user) {
        const updatedUser = await updateOrgUser(currentOrgId.toString(), user.id.toString(), {
          first_name: data.firstName,
          last_name: data.lastName,
          role_id: roleNameToId(data.role),
        });
        toast.success(`User ${updatedUser.email} updated successfully!`);
        dispatch(loadUserOrganizations());
      } else {
        if (!existsGlobally && !data.password) {
          toast.error('Password is required for new users.');
          return;
        }
        const newUser = await createOrgUser(currentOrgId.toString(), {
          email: data.email,
          first_name: data.firstName || '',
          last_name: data.lastName || '',
          role_id: roleNameToId(data.role),
          password: data.password,
        });
        toast.success(`User ${newUser.email} invited successfully!`);
        setCreatedUser(newUser);
        return;
      }
      navigate('/settings#users');
    } catch (error) {
      const action = isEditMode ? 'update' : 'invite';
      const rawMessage = (error as Error).message || 'An unknown error occurred.';
      const errorMessage = rawMessage.replace(/[\r\n]+/g, ' ').trim().slice(0, 200) || 'An unknown error occurred.';
      toast.error(['Failed to', action, 'user:', errorMessage].join(' '));
      console.error('User operation error', { action, error });
    }
  };

  if (loading) {
    return (
      <div className="p-6 bg-gray-900 text-white text-center">
        <h1 className="text-3xl font-bold">Loading User...</h1>
      </div>
    );
  }

  if (createdUser) {
    return (
      <UserOnboardingDetails
        user={createdUser}
        onContinue={() => {
          if (currentOrg?.plan_type === 'free') {
            setShowUpgradeModal(true);
          } else {
            navigate('/settings#users');
          }
        }}
      />
    );
  }

  return (
    <div className="p-6 bg-gray-900 text-white">
      <div className="max-w-4xl mx-auto">
        <div className="mb-8">
          <h1 className="text-3xl font-bold">{isEditMode ? 'Edit User' : 'Invite New User'}</h1>
          <p className="text-gray-400 mt-2">
            {isEditMode ? `Update details for ${user?.email}` : 'Invite a new user to the organization.'}
          </p>
        </div>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-6 bg-gray-800 p-8 rounded-lg">
          <Input
            label="Email Address"
            id="email"
            type="email"
            {...register('email')}
            onBlur={handleEmailCheck}
            error={errors.email?.message}
            required
            disabled={isEditMode}
            icon={checkingEmail ? (
              <svg className="animate-spin h-5 w-5 text-blue-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            ) : undefined}
            iconPosition="right"
          />
          
          {existsGlobally && !isEditMode && (
            <div className="bg-blue-900/30 border border-blue-500/50 p-4 rounded-md text-blue-200 text-sm">
              This user already has a LiveReview account. Please select a role.
            </div>
          )}

          {!existsGlobally && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <Input
                label="First Name"
                id="firstName"
                {...register('firstName')}
                error={errors.firstName?.message}
                required
              />
              <Input
                label="Last Name"
                id="lastName"
                {...register('lastName')}
                error={errors.lastName?.message}
                required
              />
            </div>
          )}

          <Select
            label="Role"
            id="role"
            {...register('role')}
            options={getRoleOptions()}
            error={errors.role?.message}
            required
          />

          {!isEditMode && !existsGlobally && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <Input
                label="Password"
                id="password"
                type="password"
                {...register('password')}
                error={errors.password?.message}
                required
              />
              <Input
                label="Confirm Password"
                id="password_confirmation"
                type="password"
                {...register('password_confirmation')}
                error={errors.password_confirmation?.message}
                required
              />
            </div>
          )}

          <div className="flex justify-end space-x-4 pt-4">
            <Button
              variant="secondary"
              type="button"
              onClick={() => navigate('/settings#users')}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? (isEditMode ? 'Updating User...' : 'Inviting User...') : (isEditMode ? 'Update User' : 'Invite User')}
            </Button>
          </div>
        </form>
      </div>

      {/* Upgrade Modal */}
      <UpgradePromptModal
        isOpen={showUpgradeModal}
        onClose={() => {
          setShowUpgradeModal(false);
          navigate('/settings#users');
        }}
        reason="MEMBER_ACTIVATION"
      />
    </div>
  );
};

export default UserForm;
