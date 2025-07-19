import React, { useState, useEffect } from 'react';
import { Card, Input, Button, Alert, Icons, PageHeader } from '../../components/UIPrimitives';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { loginAdmin, clearError } from '../../store/Auth/reducer';

const Login: React.FC = () => {
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);
  
  const dispatch = useAppDispatch();
  const { isLoading, error } = useAppSelector((state) => state.Auth);
  
  // Clear validation error when user starts typing
  useEffect(() => {
    if (validationError) {
      setValidationError(null);
    }
  }, [username, password]);
  
  // Clear API error when user starts typing
  useEffect(() => {
    if (error) {
      dispatch(clearError());
    }
  }, [username, password, dispatch, error]);
  
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // Validate
    if (username !== 'admin') {
      setValidationError('Username must be "admin"');
      return;
    }
    
    if (!password) {
      setValidationError('Password is required');
      return;
    }
    
    console.log('Login form submitted with password:', password);
    
    // Submit
    const result = dispatch(loginAdmin(password));
    console.log('Dispatched loginAdmin action, result:', result);
    
    // We can log when the promise resolves to see if it succeeds
    result.then(
      (action) => console.log('Login action fulfilled:', action),
      (error) => console.error('Login action rejected:', error)
    );
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
            {(validationError || error) && (
              <Alert 
                variant="error" 
                icon={<Icons.Error />}
                className="mb-4"
              >
                {validationError || error}
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
                onChange={(e) => setPassword(e.target.value)}
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
