module.exports = {
  apps : [{
    name: 'livereview-api',
    script: './livereview',
    args: 'api',
    watch: false
  }, {
    name: 'livereview-ui',
    script: './livereview',
    args: 'ui',
    watch: false
  }]
};
