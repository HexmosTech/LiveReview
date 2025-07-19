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
    <div className="container mx-auto px-4 py-8 flex flex-col items-center justify-center min-h-[80vh]">
      <div className="max-w-md w-full">
        <div className="flex justify-center mb-8">
          <img src="/assets/logo.svg" alt="LiveReview Logo" className="h-16 w-auto" />
        </div>
        
        <PageHeader 
          title="Administrator Login" 
          description="Please log in to access your LiveReview instance"
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
                label="Username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="admin"
              />
              
              <Input
                label="Password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter your password"
              />
              
              <div className="pt-2">
                <Button 
                  type="submit"
                  variant="primary"
                  fullWidth
                  isLoading={isLoading}
                  disabled={isLoading}
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
