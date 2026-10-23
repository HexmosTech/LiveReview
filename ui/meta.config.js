// Page title, description and link-preview tags baked into index.html (html-webpack-plugin `meta`). Link previews
// (Slack, X, LinkedIn, WhatsApp) and search engines read only this static HTML, never the React app, so these are
// what people see when a LiveReview URL is shared, e.g. https://livereview.hexmos.com/gh-login?src=launch-trial.
//
// Cloud builds get the canonical URL and are indexable. Self-hosted builds keep the same title and description but
// ask search engines not to index them: a company's own instance is private.

const siteURL = 'https://livereview.hexmos.com';
const title = 'LiveReview: Auto-approve up to 50% of your pull requests, safely';
// Search engines show about 155 characters of the description.
const description =
    'Risk-aware AI code review: LiveReview scores every pull request, auto-approves the safe ones and sends only risky changes to your reviewers.';
const image = `${siteURL}/public/og-image.png`; // 1200x630
const imageAlt = 'LiveReview: auto-approve up to 50% of your pull requests, safely';

// Open Graph tags must use the property attribute (Facebook and LinkedIn ignore name="og:...").
const og = (property, content) => ({ property, content });

module.exports = function buildMetaConfig(isCloud) {
    return {
        title,
        siteURL: isCloud ? siteURL : '',
        meta: {
            description,
            keywords:
                'AI code review, pull request review, auto-approve pull requests, risk scoring, GitHub App, GitLab merge request review, code review automation',
            author: 'Hexmos',
            robots: isCloud ? 'index, follow' : 'noindex, nofollow',
            'og:type': og('og:type', 'website'),
            'og:site_name': og('og:site_name', 'LiveReview'),
            'og:title': og('og:title', title),
            'og:description': og('og:description', description),
            ...(isCloud ? { 'og:url': og('og:url', siteURL) } : {}),
            'og:image': og('og:image', image),
            'og:image:width': og('og:image:width', '1200'),
            'og:image:height': og('og:image:height', '630'),
            'og:image:alt': og('og:image:alt', imageAlt),
            'twitter:card': 'summary_large_image',
            'twitter:title': title,
            'twitter:description': description,
            'twitter:image': image,
            'twitter:image:alt': imageAlt,
        },
    };
};
