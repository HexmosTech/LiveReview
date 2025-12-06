import React, { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useOrgContext } from '../../hooks/useOrgContext';
import apiClient from '../../api/apiClient';
import toast from 'react-hot-toast';

type Subscription = {
  id: number;
  razorpay_subscription_id: string;
  plan_type: string;
  quantity: number;
  assigned_seats: number;
  status: string;
};

type OrgMember = {
  id: number;
  email: string;
  role: string;
  plan_type: string;
  license_expires_at: string | null;
};

const LicenseAssignment: React.FC = () => {
  const navigate = useNavigate();
  const { id: subscriptionId } = useParams<{ id: string }>();
  const { currentOrgId, currentOrg, currentOrgMembers, loadMembers } = useOrgContext();
  
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [processing, setProcessing] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedMembers, setSelectedMembers] = useState<Set<number>>(new Set());
  const [bulkProcessing, setBulkProcessing] = useState(false);

  useEffect(() => {
    if (!currentOrgId || !subscriptionId) {
      setError('Missing organization or subscription context');
      setLoading(false);
      return;
    }

    loadData();
  }, [currentOrgId, subscriptionId]);

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);

      // Load subscription details
      const subResponse = await apiClient.get<Subscription>(`/subscriptions/${subscriptionId}`);
      setSubscription(subResponse);

      // Load org members
      if (currentOrgId) {
        await loadMembers(currentOrgId);
      }

      // Fetch member details with license info
      const membersResponse = await apiClient.get<{ members: OrgMember[] }>(
        `/orgs/${currentOrgId}/users`
      );
      setMembers(membersResponse.members || []);
    } catch (err: any) {
      setError(err.message || 'Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  const handleAssign = async (userId: number) => {
    if (!subscriptionId || !subscription) return;

    // Check if seats available
    if (subscription.assigned_seats >= subscription.quantity) {
      toast.error('No available seats. Increase subscription quantity first.');
      return;
    }

    try {
      setProcessing(userId);
      await apiClient.post(`/subscriptions/${subscriptionId}/assign`, {
        user_id: userId,
      });
      
      toast.success('License assigned successfully');
      loadData(); // Reload to get updated state
    } catch (err: any) {
      toast.error(err.message || 'Failed to assign license');
    } finally {
      setProcessing(null);
    }
  };

  const handleRevoke = async (userId: number) => {
    if (!subscriptionId) return;

    if (!confirm('Are you sure you want to revoke this license? The user will lose Team plan access.')) {
      return;
    }

    try {
      setProcessing(userId);
      await apiClient.delete(`/subscriptions/${subscriptionId}/users/${userId}`);
      
      toast.success('License revoked successfully');
      loadData(); // Reload to get updated state
    } catch (err: any) {
      toast.error(err.message || 'Failed to revoke license');
    } finally {
      setProcessing(null);
    }
  };

  const isLicensed = (member: OrgMember) => {
    return member.plan_type === 'team' && member.license_expires_at;
  };

  const toggleMember = (userId: number) => {
    setSelectedMembers(prev => {
      const next = new Set(prev);
      if (next.has(userId)) {
        next.delete(userId);
      } else {
        next.add(userId);
      }
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selectedMembers.size === members.length) {
      setSelectedMembers(new Set());
    } else {
      setSelectedMembers(new Set(members.map(m => m.id)));
    }
  };

  const handleBulkAssign = async () => {
    if (!subscriptionId || !subscription || selectedMembers.size === 0) return;

    const unlicensedSelected = Array.from(selectedMembers).filter(id => {
      const member = members.find(m => m.id === id);
      return member && !isLicensed(member);
    });

    if (unlicensedSelected.length === 0) {
      toast.error('No unlicensed members selected');
      return;
    }

    const availableSeats = subscription.quantity - subscription.assigned_seats;
    if (unlicensedSelected.length > availableSeats) {
      toast.error(`Only ${availableSeats} seat(s) available, but ${unlicensedSelected.length} selected`);
      return;
    }

    setBulkProcessing(true);
    let successCount = 0;
    let errorCount = 0;

    for (const userId of unlicensedSelected) {
      try {
        await apiClient.post(`/subscriptions/${subscriptionId}/assign`, {
          user_id: userId,
        });
        successCount++;
      } catch (err: any) {
        errorCount++;
      }
    }

    setBulkProcessing(false);
    setSelectedMembers(new Set());

    if (successCount > 0) {
      toast.success(`${successCount} license(s) assigned successfully`);
    }
    if (errorCount > 0) {
      toast.error(`${errorCount} assignment(s) failed`);
    }

    loadData();
  };

  const handleBulkRevoke = async () => {
    if (!subscriptionId || selectedMembers.size === 0) return;

    const licensedSelected = Array.from(selectedMembers).filter(id => {
      const member = members.find(m => m.id === id);
      return member && isLicensed(member);
    });

    if (licensedSelected.length === 0) {
      toast.error('No licensed members selected');
      return;
    }

    if (!confirm(`Are you sure you want to revoke ${licensedSelected.length} license(s)? Users will lose Team plan access.`)) {
      return;
    }

    setBulkProcessing(true);
    let successCount = 0;
    let errorCount = 0;

    for (const userId of licensedSelected) {
      try {
        await apiClient.delete(`/subscriptions/${subscriptionId}/users/${userId}`);
        successCount++;
      } catch (err: any) {
        errorCount++;
      }
    }

    setBulkProcessing(false);
    setSelectedMembers(new Set());

    if (successCount > 0) {
      toast.success(`${successCount} license(s) revoked successfully`);
    }
    if (errorCount > 0) {
      toast.error(`${errorCount} revocation(s) failed`);
    }

    loadData();
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-8 px-4">
        <div className="max-w-4xl mx-auto">
          <div className="flex items-center justify-center py-20">
            <div className="inline-block w-8 h-8 border-4 border-slate-600 border-t-blue-500 rounded-full animate-spin" />
          </div>
        </div>
      </div>
    );
  }

  if (error || !subscription) {
    return (
      <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-8 px-4">
        <div className="max-w-4xl mx-auto">
          <div className="mb-6 p-4 bg-red-500/10 border border-red-500/40 rounded-lg">
            <p className="text-red-300">{error || 'Subscription not found'}</p>
          </div>
          <button
            onClick={() => navigate('/subscribe/manage')}
            className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-white rounded-lg transition-colors"
          >
            Back to Subscriptions
          </button>
        </div>
      </div>
    );
  }

  const availableSeats = subscription.quantity - subscription.assigned_seats;

  return (
    <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-8 px-4">
      <div className="max-w-4xl mx-auto">
        {/* Header */}
        <div className="mb-8">
          <button
            onClick={() => navigate('/subscribe/manage')}
            className="inline-flex items-center text-slate-400 hover:text-white mb-4 transition-colors"
          >
            <svg className="w-5 h-5 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
            Back to Subscriptions
          </button>
          <h1 className="text-3xl font-bold text-white mb-2">Assign Team Licenses</h1>
          <p className="text-slate-400">
            Manage license assignments for your team {currentOrg && `in ${currentOrg.name}`}
          </p>
        </div>

        {/* Subscription Info */}
        <div className="bg-slate-800 rounded-xl border border-slate-700 p-6 mb-8">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-6">
            <div>
              <div className="text-slate-400 text-sm mb-1">Total Seats</div>
              <div className="text-2xl font-bold text-white">{subscription.quantity}</div>
            </div>
            <div>
              <div className="text-slate-400 text-sm mb-1">Assigned</div>
              <div className="text-2xl font-bold text-emerald-400">{subscription.assigned_seats}</div>
            </div>
            <div>
              <div className="text-slate-400 text-sm mb-1">Available</div>
              <div className="text-2xl font-bold text-blue-400">{availableSeats}</div>
            </div>
          </div>

          {availableSeats === 0 && (
            <div className="mt-4 p-3 bg-orange-500/10 border border-orange-500/40 rounded-lg">
              <p className="text-orange-300 text-sm">
                All seats are assigned. Increase your subscription quantity to assign more licenses.
              </p>
            </div>
          )}
        </div>

        {/* Members List */}
        <div className="bg-slate-800 rounded-xl border border-slate-700 overflow-hidden">
          <div className="p-6 border-b border-slate-700">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-xl font-semibold text-white">Team Members</h2>
                <p className="text-sm text-slate-400 mt-1">
                  Assign or revoke team licenses for organization members
                </p>
              </div>
              {selectedMembers.size > 0 && (
                <div className="flex items-center gap-3">
                  <span className="text-sm text-slate-400">
                    {selectedMembers.size} selected
                  </span>
                  <button
                    onClick={handleBulkAssign}
                    disabled={bulkProcessing}
                    className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {bulkProcessing ? 'Processing...' : 'Assign Selected'}
                  </button>
                  <button
                    onClick={handleBulkRevoke}
                    disabled={bulkProcessing}
                    className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {bulkProcessing ? 'Processing...' : 'Revoke Selected'}
                  </button>
                </div>
              )}
            </div>
          </div>

          {members.length === 0 ? (
            <div className="p-12 text-center">
              <p className="text-slate-400">No team members found</p>
            </div>
          ) : (
            <div>
              {/* Select All Header */}
              <div className="p-4 border-b border-slate-700 bg-slate-900/40 flex items-center">
                <div className="w-4 h-4 flex items-center justify-center flex-shrink-0">
                  <input
                    type="checkbox"
                    checked={selectedMembers.size === members.length && members.length > 0}
                    onChange={toggleSelectAll}
                    className="w-4 h-4 text-blue-600 bg-slate-700 border-slate-600 rounded focus:ring-blue-500 focus:ring-2"
                  />
                </div>
                <span className="ml-4 text-sm font-medium text-slate-300">
                  Select All ({members.length})
                </span>
              </div>

              {/* Member Rows */}
              <div className="divide-y divide-slate-700">
                {members.map((member) => {
                  const hasLicense = isLicensed(member);
                  const isProcessingThis = processing === member.id;
                  const isSelected = selectedMembers.has(member.id);

                  return (
                    <div key={member.id} className="p-4 hover:bg-slate-900/40 transition-colors flex items-center gap-4">
                      <div className="w-4 h-4 flex items-center justify-center flex-shrink-0">
                        <input
                          type="checkbox"
                          checked={isSelected}
                          onChange={() => toggleMember(member.id)}
                          disabled={bulkProcessing || isProcessingThis}
                          className="w-4 h-4 text-blue-600 bg-slate-700 border-slate-600 rounded focus:ring-blue-500 focus:ring-2 disabled:opacity-50"
                        />
                      </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-3 mb-1 flex-wrap">
                            <h3 className="text-white font-medium truncate">{member.email}</h3>
                            <span className="px-2 py-1 text-xs font-semibold rounded border bg-slate-700/40 text-slate-300 border-slate-600 flex-shrink-0">
                              {member.role}
                            </span>
                            {hasLicense && (
                              <span className="px-2 py-1 text-xs font-semibold rounded border bg-emerald-500/10 text-emerald-400 border-emerald-500/40 flex-shrink-0">
                                Licensed
                              </span>
                            )}
                          </div>
                          {hasLicense && member.license_expires_at && (
                            <p className="text-sm text-slate-400">
                              Expires: {new Date(member.license_expires_at).toLocaleDateString()}
                            </p>
                          )}
                        </div>

                        <div className="flex-shrink-0">
                          {hasLicense ? (
                            <button
                              onClick={() => handleRevoke(member.id)}
                              disabled={isProcessingThis || bulkProcessing}
                              className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed min-w-[100px]"
                            >
                              {isProcessingThis ? 'Revoking...' : 'Revoke'}
                            </button>
                          ) : (
                            <button
                              onClick={() => handleAssign(member.id)}
                              disabled={isProcessingThis || availableSeats === 0 || bulkProcessing}
                              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed min-w-[100px]"
                            >
                              {isProcessingThis ? 'Assigning...' : 'Assign License'}
                            </button>
                          )}
                        </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default LicenseAssignment;
