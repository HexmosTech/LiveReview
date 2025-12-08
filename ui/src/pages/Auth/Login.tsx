import React from 'react';
import Cloud from './Cloud';
import SelfHosted from './SelfHosted';
import { isCloudMode } from '../../utils/deploymentMode';

const Login: React.FC = () => {
  return isCloudMode() ? <Cloud /> : <SelfHosted />;
};

export default Login;
