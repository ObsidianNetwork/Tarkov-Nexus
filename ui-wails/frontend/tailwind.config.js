/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Obsidian Primary Brand Colors
        'primary-purple': '#5865F2',
        'dark-purple': '#4752C4',
        'light-purple': '#7289DA',
        'electric-purple': '#8B5FBF',

        // Accent Colors
        'neon-green': '#00D4AA',
        'warning': '#FEE75C',
        'error': '#ED4245',

        // Dark Theme Background System
        'bg-dark': '#0C0E14',
        'bg-darker': '#06080B',
        'bg-card': '#1A1D26',
        'bg-hover': '#232631',

        // Text Colors
        'text-primary': '#FFFFFF',
        'text-secondary': '#B9BBBE',
        'text-muted': '#72767D',

        // Border & Accent
        'border-color': '#2A2D38',
      },
      animation: {
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        'gradient-shift': 'gradientShift 3s ease infinite',
        'fade-in-up': 'fadeInUp 0.6s ease forwards',
        'fade-in-left': 'fadeInLeft 1s ease forwards',
        'fade-in-right': 'fadeInRight 1s ease forwards',
        'float': 'float 20s infinite linear',
        'ripple': 'ripple 0.6s linear',
      },
      keyframes: {
        gradientShift: {
          '0%, 100%': { backgroundPosition: '0% 50%' },
          '50%': { backgroundPosition: '100% 50%' },
        },
        fadeInUp: {
          from: { opacity: '0', transform: 'translateY(20px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        fadeInLeft: {
          from: { opacity: '0', transform: 'translateX(-30px)' },
          to: { opacity: '1', transform: 'translateX(0)' },
        },
        fadeInRight: {
          from: { opacity: '0', transform: 'translateX(30px)' },
          to: { opacity: '1', transform: 'translateX(0)' },
        },
        float: {
          from: { transform: 'translateY(100vh) translateX(0)' },
          to: { transform: 'translateY(-100px) translateX(100px)' },
        },
        ripple: {
          to: { transform: 'scale(4)', opacity: '0' },
        },
      },
      backdropBlur: {
        xs: '2px',
      },
      boxShadow: {
        'glow-sm': '0 0 10px rgba(88, 101, 242, 0.3)',
        'glow-md': '0 0 20px rgba(88, 101, 242, 0.4)',
        'glow-lg': '0 0 30px rgba(88, 101, 242, 0.5)',
        'glow-green': '0 0 20px rgba(0, 212, 170, 0.4)',
      },
    },
  },
  plugins: [
    require('@tailwindcss/forms'),
  ],
}
