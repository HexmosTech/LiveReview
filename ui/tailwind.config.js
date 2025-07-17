module.exports = {
  content: [ 
    './src/**/*.html',
    './src/**/*.{js,jsx,ts,tsx}' 
  ],
  darkMode: 'media', // or 'media' or 'class'
  theme: {
    extend: {
      colors: {
        livereview: {
          DEFAULT: '#181C24', // dark background
          accent: '#3B82F6', // blue accent (buttons, headings)
          purple: '#7C3AED', // purple accent (cards, highlights)
          green: '#22C55E', // green accent (success, highlights)
          cardPurple: '#7C3AED',
          cardBlue: '#2563EB',
          cardGreen: '#16A34A',
          cardText: '#F3F4F6',
        },
        background: '#181C24',
        accent: '#3B82F6',
        purple: '#7C3AED',
        green: '#22C55E',
        cardPurple: '#7C3AED',
        cardBlue: '#2563EB',
        cardGreen: '#16A34A',
        cardText: '#F3F4F6',
      },
    },
  },
  variants: {
    extend: {},
  },
  plugins: [],
}
