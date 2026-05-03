module.exports = {
  apps : [{
    name: 'livereview-api',
    script: './livereview',
    args: 'api',
    cwd: __dirname,
    watch: false
  }, {
    name: 'livereview-ui',
    script: './livereview',
    args: ["ui", "--port", "8081"],
    cwd: __dirname,
    env: {
      LIVEREVIEW_REVERSE_PROXY: "true"
    },
    watch: false
  }]
};
