import React, { useState } from 'react';
import { Card, Input, Button, Alert, Icons } from '../../components/UIPrimitives';
import { verifyAdminPassword } from '../../api/auth';

const Login: React.FC = () => {
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    
    // Validate input
    if (!password) {
      setError('Password is required');
      return;
    }
    
    // Show loading state
    setIsLoading(true);
    
    try {
      // Call API directly
      const response = await verifyAdminPassword(password);
      
      // Handle response
      if (response.success) {
        // Store authentication in localStorage
        localStorage.setItem('authPassword', password);
        // Redirect or update app state
        window.location.href = '/'; // or use react-router navigation
        console.log('Login successful');
      } else {
        setError('Invalid password');
      }
    } catch (err) {
      // Handle error
      setError('Login failed. Please try again.');
      console.error('Login error:', err);
    } finally {
      setIsLoading(false);
    }
  };
  
  return (
    <div className="min-h-screen bg-slate-900 flex flex-col items-center justify-center px-4 py-12">
      <div className="max-w-md w-full space-y-8">
        <div className="text-center">
          <img src="assets/logo.svg" alt="LiveReview Logo" className="mx-auto h-20 w-auto" />
          <h2 className="mt-6 text-3xl font-extrabold text-white">Administrator Login</h2>
          <p className="mt-2 text-sm text-slate-300">Please log in to access your LiveReview instance</p>
        </div>
        
        <Card className="shadow-xl border border-slate-700 rounded-xl overflow-hidden">
          <form onSubmit={handleSubmit} className="space-y-6">
            {error && (
              <Alert 
                variant="error" 
                icon={<Icons.Error />}
                className="mb-4"
              >
                {error}
              </Alert>
            )}
            
            <div className="space-y-5">
              <Input
                label="Username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="admin"
                className="bg-slate-700 border-slate-600 text-white"
              />
              
              <Input
                label="Password"
                type="password"
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                  if (error) setError(null);
                }}
                placeholder="Enter your password"
                className="bg-slate-700 border-slate-600 text-white"
              />
              
              <div className="pt-2">
                <Button 
                  type="submit"
                  variant="primary"
                  fullWidth
                  isLoading={isLoading}
                  disabled={isLoading}
                  className="py-3 font-semibold"
                >
                  Log In
                </Button>
              </div>
            </div>
          </form>
        </Card>
      </div>
    </div>
  );
};

export default Login;
