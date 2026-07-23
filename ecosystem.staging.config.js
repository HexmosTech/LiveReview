module.exports = {
  apps : [{
    name: 'livereview-staging-api',
    script: './livereview',
    args: 'api',
    cwd: __dirname,
    watch: false
  }, {
    name: 'livereview-staging-worker',
    script: './livereview',
    args: 'worker',
    cwd: __dirname,
    watch: false
  },
   {
    name: 'livereview-staging-ui',
    script: './livereview',
    args: 'ui',
    cwd: __dirname,
    env: {
      LIVEREVIEW_REVERSE_PROXY: "true"
    },
    watch: false
  }]
};
