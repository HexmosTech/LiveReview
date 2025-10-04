// Shared configuration for all tests
export const SHARED_CONFIG = {
  // Site URL - update this to match your environment
  // BASE_URL: 'https://livereview.hexmos.site/',
  BASE_URL: 'https://manual-talent.apps.hexmos.com/',
  
  // Super admin credentials (for login)
  SUPER_ADMIN_EMAIL: 'shrijith@hexmos.com',
  SUPER_ADMIN_PASSWORD: 'MegaSuperAdmin@123',
  
  // Owner user credentials (for login)
  OWNER_EMAIL: 'general@hexmos.com',
  OWNER_PASSWORD: 'MegaGeneral@123'
};

// GitHub connector specific config
export const GITHUB_CONFIG = {
  CONNECTOR_NAME: 'GH2028',
  GITHUB_USERNAME: 'hexmos', // GitHub username if required
  GITHUB_TOKEN: 'REDACTED_GITHUB_PAT_3'
};