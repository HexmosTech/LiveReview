module.exports = {
  apps : [{
    name: 'livereview-api',
    script: './livereview',
    args: 'api',
    watch: false
  }, {
    name: 'livereview-ui',
    script: './livereview',
    args: ["ui", "--port", "8081"],
    env: {
      LIVEREVIEW_REVERSE_PROXY: "true"
    },
    watch: false
  }]
};
