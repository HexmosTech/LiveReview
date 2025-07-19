import React, { useState, useEffect } from 'react';
import { Card, Input, Button, Alert, Icons, PageHeader } from '../../components/UIPrimitives';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { setInitialPassword, clearError } from '../../store/Auth/reducer';

const SetPassword: React.FC = () => {
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [validationError, setValidationError] = useState<string | null>(null);
  
  const dispatch = useAppDispatch();
  const { isLoading, error } = useAppSelector((state) => state.Auth);
  
  // Clear validation error when user starts typing
  useEffect(() => {
    if (validationError) {
      setValidationError(null);
    }
  }, [password, confirmPassword]);
  
  // Clear API error when user starts typing
  useEffect(() => {
    if (error) {
      dispatch(clearError());
    }
  }, [password, confirmPassword, dispatch, error]);
  
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    // Validate
    if (!password) {
      setValidationError('Password is required');
      return;
    }
    
    if (password.length < 8) {
      setValidationError('Password must be at least 8 characters long');
      return;
    }
    
    if (password !== confirmPassword) {
      setValidationError('Passwords do not match');
      return;
    }
    
    // Submit
    dispatch(setInitialPassword(password));
  };
  
  return (
    <div className="min-h-screen bg-slate-900 flex flex-col items-center justify-center px-4 py-12">
      <div className="max-w-md w-full space-y-8">
        <div className="text-center">
          <img src="assets/logo.svg" alt="LiveReview Logo" className="mx-auto h-20 w-auto" />
          <h2 className="mt-6 text-3xl font-extrabold text-white">Set Administrator Password</h2>
          <p className="mt-2 text-sm text-slate-300">Set up your LiveReview administrator password to secure your installation</p>
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
                label="Administrator Username"
                value="admin"
                disabled
                helperText="The default administrator username is 'admin'"
                className="bg-slate-700 border-slate-600 text-white opacity-70"
              />
              
              <Input
                label="Password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter a secure password"
                helperText="Password must be at least 8 characters long"
                className="bg-slate-700 border-slate-600 text-white"
              />
              
              <Input
                label="Confirm Password"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Confirm your password"
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
                  Set Password & Continue
                </Button>
              </div>
            </div>
          </form>
        </Card>
      </div>
    </div>
  );
};

export default SetPassword;
