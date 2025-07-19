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
    <div className="container mx-auto px-4 py-8 flex flex-col items-center justify-center min-h-[80vh]">
      <div className="max-w-md w-full">
        <div className="flex justify-center mb-8">
          <img src="/assets/logo.svg" alt="LiveReview Logo" className="h-16 w-auto" />
        </div>
        
        <PageHeader 
          title="Set Administrator Password" 
          description="Set up your LiveReview administrator password to secure your installation"
          className="text-center mb-6"
        />
        
        <Card>
          <form onSubmit={handleSubmit}>
            {(validationError || error) && (
              <Alert 
                variant="error" 
                icon={<Icons.Error />}
                className="mb-4"
              >
                {validationError || error}
              </Alert>
            )}
            
            <div className="space-y-6">
              <Input
                label="Administrator Username"
                value="admin"
                disabled
                helperText="The default administrator username is 'admin'"
              />
              
              <Input
                label="Password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter a secure password"
                helperText="Password must be at least 8 characters long"
              />
              
              <Input
                label="Confirm Password"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Confirm your password"
              />
              
              <div className="pt-2">
                <Button 
                  type="submit"
                  variant="primary"
                  fullWidth
                  isLoading={isLoading}
                  disabled={isLoading}
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
