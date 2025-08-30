const package = require('./package.json');

const title = 'LiveReview' || package.name;
const description = `A powerful tool for automated code reviews` || package.description;
const homepage = '' || package.homepage;
const thumbnail = `${homepage}/public/thumbnail.jpg`;

module.exports = {
    title,
    meta: {
        language: 'en',
        description,
        keywords: 'code review, automation, gitlab, github, continuous integration',
        author: 'LiveReview Team',
        /**
         * Open Graph tags optimize how your pages appear when shared on social media platform
         */
        'og:title': title,
        'og:description': description,
        'og:url': homepage,
        'og:image': thumbnail,
        /**
         * Similar to Open Graph tags, but specifically for Twitter. They enhance the way your content appears when shared on Twitter.
         */
        'twitter:card': 'summary_large_image',
        'twitter:title': title,
        'twitter:description': description,
        'twitter:image': thumbnail,
        /**
         * Use index (default) to allow indexing, or noindex for sensitive/non-public pages.
         * Use follow to allow crawling of links on the page.
         */
        'robots': 'index, follow',
    },
};