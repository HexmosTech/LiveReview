import React from 'react';
import Cloud from './Cloud';
import SelfHosted from './SelfHosted';

const Login: React.FC = () => {
  const isCloud = (process.env.LIVEREVIEW_IS_CLOUD || '').toString().toLowerCase() === 'true';

  return isCloud ? <Cloud /> : <SelfHosted />;
};

export default Login;
