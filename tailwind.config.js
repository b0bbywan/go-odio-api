/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./ui/templates/**/*.gohtml",
  ],
  theme: {
    extend: {
      colors: {
        // Remap zinc → forest palette (dark green-tinted)
        zinc: {
          100: '#e4f0e5',
          200: '#c4dcc8',
          300: '#9ab8a2',
          400: '#6a9472',
          500: '#3a5440',
          600: '#243629',
          700: '#1a2a1e',
          800: '#131d16',
          900: '#0d1410',
          950: '#060c08',
        },
        leaf: '#4a7c59',
        moss:  '#2a4e35',
        gold:  '#c8963a',
      },
    },
  },
  plugins: [],
}
