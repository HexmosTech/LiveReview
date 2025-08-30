import React, { useState } from 'react';
import { useSelector } from 'react-redux';
import { Button } from '../components/UIPrimitives';

interface TestResult {
  endpoint: string;
  status: 'loading' | 'success' | 'error';
  response?: any;
  error?: string;
}

export const MiddlewareTestPage: React.FC = () => {
  const [testResults, setTestResults] = useState<Record<string, TestResult>>({});
  const auth = useSelector((state: any) => state.Auth);

  const runTest = async (testName: string, endpoint: string, description: string) => {
    setTestResults(prev => ({
      ...prev,
      [testName]: { endpoint, status: 'loading' }
    }));

    try {
      const response = await fetch(`/api/v1${endpoint}`, {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
          ...(auth.accessToken ? { 'Authorization': `Bearer ${auth.accessToken}` } : {})
        }
      });

      const data = await response.json();

      if (response.ok) {
        setTestResults(prev => ({
          ...prev,
          [testName]: { 
            endpoint, 
            status: 'success', 
            response: data 
          }
        }));
      } else {
        setTestResults(prev => ({
          ...prev,
          [testName]: { 
            endpoint, 
            status: 'error', 
            error: `${response.status}: ${data.error || data.message || 'Unknown error'}` 
          }
        }));
      }
    } catch (error) {
      setTestResults(prev => ({
        ...prev,
        [testName]: { 
          endpoint, 
          status: 'error', 
          error: error instanceof Error ? error.message : 'Network error' 
        }
      }));
    }
  };

  const getOrgId = () => {
    if (auth.organizations && auth.organizations.length > 0) {
      return auth.organizations[0].id;
    }
    return '1'; // fallback
  };

  const renderTestResult = (testName: string, result: TestResult) => {
    const { status, response, error } = result;
    
    return (
      <div className="mt-3 p-3 rounded border">
        <div className="font-mono text-sm text-gray-600 mb-2">
          {result.endpoint}
        </div>
        
        {status === 'loading' && (
          <div className="text-blue-600">⏳ Testing...</div>
        )}
        
        {status === 'success' && (
          <div>
            <div className="text-green-600 font-semibold mb-2">
              ✅ {response.message}
            </div>
            <pre className="bg-gray-100 p-2 rounded text-xs overflow-x-auto">
              {JSON.stringify(response, null, 2)}
            </pre>
          </div>
        )}
        
        {status === 'error' && (
          <div className="text-red-600">
            <div className="font-semibold mb-1">❌ Failed</div>
            <div className="text-sm">{error}</div>
          </div>
        )}
      </div>
    );
  };

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <h1 className="text-2xl font-bold mb-6">🧪 Middleware Testing Page</h1>
      
      <div className="mb-6 p-4 bg-blue-50 rounded">
        <h2 className="font-semibold mb-2">Current Auth State:</h2>
        <div className="text-sm">
          <div>Authenticated: {auth.isAuthenticated ? '✅ Yes' : '❌ No'}</div>
          <div>User: {auth.user?.email || 'None'}</div>
          <div>Organizations: {auth.organizations?.map((org: any) => `${org.name} (${org.role})`).join(', ') || 'None'}</div>
          <div>Access Token: {auth.accessToken ? '✅ Present' : '❌ Missing'}</div>
        </div>
      </div>

      <div className="space-y-6">
        {/* Public Test */}
        <div className="border rounded p-4">
          <h3 className="text-lg font-semibold mb-2">1. Public Endpoint Test</h3>
          <p className="text-gray-600 mb-3">
            Should work without authentication. Tests that public routes are accessible.
          </p>
          <Button
            onClick={() => runTest('public', '/test/public', 'Public endpoint test')}
            disabled={testResults.public?.status === 'loading'}
          >
            Test Public Endpoint
          </Button>
          {testResults.public && renderTestResult('public', testResults.public)}
        </div>

        {/* Protected Test */}
        <div className="border rounded p-4">
          <h3 className="text-lg font-semibold mb-2">2. Protected Endpoint Test</h3>
          <p className="text-gray-600 mb-3">
            Requires valid JWT token. Should work if logged in, fail if not.
          </p>
          <Button
            onClick={() => runTest('protected', '/test/protected', 'Protected endpoint test')}
            disabled={testResults.protected?.status === 'loading'}
          >
            Test Protected Endpoint
          </Button>
          {testResults.protected && renderTestResult('protected', testResults.protected)}
        </div>

        {/* Token Info Test */}
        <div className="border rounded p-4">
          <h3 className="text-lg font-semibold mb-2">3. Token Info Test</h3>
          <p className="text-gray-600 mb-3">
            Shows your current token information for debugging.
          </p>
          <Button
            onClick={() => runTest('tokenInfo', '/test/token-info', 'Token info test')}
            disabled={testResults.tokenInfo?.status === 'loading'}
          >
            Test Token Info
          </Button>
          {testResults.tokenInfo && renderTestResult('tokenInfo', testResults.tokenInfo)}
        </div>

        {/* Org-Scoped Test */}
        <div className="border rounded p-4">
          <h3 className="text-lg font-semibold mb-2">4. Organization-Scoped Test</h3>
          <p className="text-gray-600 mb-3">
            Requires valid JWT + organization membership. Tests org-scoped middleware.
          </p>
          <Button
            onClick={() => runTest('orgScoped', `/orgs/${getOrgId()}/test`, 'Org-scoped test')}
            disabled={testResults.orgScoped?.status === 'loading'}
          >
            Test Org-Scoped Endpoint (Org ID: {getOrgId()})
          </Button>
          {testResults.orgScoped && renderTestResult('orgScoped', testResults.orgScoped)}
        </div>

        {/* Super Admin Test */}
        <div className="border rounded p-4">
          <h3 className="text-lg font-semibold mb-2">5. Super Admin Test</h3>
          <p className="text-gray-600 mb-3">
            Requires super admin role. Should work only for super admins.
          </p>
          <Button
            onClick={() => runTest('superAdmin', '/admin/test', 'Super admin test')}
            disabled={testResults.superAdmin?.status === 'loading'}
          >
            Test Super Admin Endpoint
          </Button>
          {testResults.superAdmin && renderTestResult('superAdmin', testResults.superAdmin)}
        </div>
      </div>

      <div className="mt-8 p-4 bg-gray-50 rounded">
        <h3 className="font-semibold mb-2">Expected Results:</h3>
        <ul className="text-sm space-y-1">
          <li>🟢 <strong>Public:</strong> Should always work (200 OK)</li>
          <li>🟡 <strong>Protected:</strong> Works if logged in (200 OK), fails if not (401 Unauthorized)</li>
          <li>🟡 <strong>Token Info:</strong> Works if logged in, shows token details</li>
          <li>🔵 <strong>Org-Scoped:</strong> Works if user belongs to the organization</li>
          <li>🔴 <strong>Super Admin:</strong> Works only for super admin users</li>
        </ul>
      </div>

      <div className="mt-6">
        <Button
          onClick={() => setTestResults({})}
          variant="outline"
        >
          Clear All Results
        </Button>
      </div>
    </div>
  );
};