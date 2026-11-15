/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./**/*.templ",
    "./**/*.html",
    "./internal/ui/*.templ",
    "./cmd/**/*.go",
    "./internal/**/*.go",
  ],
  theme: {
    extend: {
      colors: {
        neonPink: "#ff00aa",
        neonCyan: "#00f3ff",
        deepPurple: {
          950: "#0c001a",
          900: "#140028",
          800: "#1a0033",
          700: "#2a0055",
        },
      },
      fontFamily: {
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "Monaco", "Consolas", "Liberation Mono", "Courier New", "monospace"],
      },
      boxShadow: {
        neon: "0 0 15px -3px rgb(0 243 255), 0 0 6px -4px rgb(255 0 170)",
        "neon-pink": "0 0 5px #ff00aa, 0 0 10px #ff00aa, 0 0 20px #ff00aa, 0 0 40px #ff00aa",
        "neon-cyan": "0 0 5px #00f3ff, 0 0 10px #00f3ff, 0 0 20px #00f3ff, 0 0 40px #00f3ff",
        glow: "0 0 25px -4px rgb(255 0 170)",
      },
      backgroundImage: {
        "grid-horizon": "linear-gradient(#1a0033 1px, transparent 1px), linear-gradient(90deg, #1a0033 1px, transparent 1px)",
      },
      animation: {
        "grid-move": "grid-move 20s linear infinite",
        "vhs-scan": "vhs-scan 4s linear infinite",
      },
      keyframes: {
        "grid-move": {
          "0%": { backgroundPosition: "0 0" },
          "100%": { backgroundPosition: "50px 50px" },
        },
        "vhs-scan": {
          "0%": { backgroundPosition: "0 0" },
          "100%": { backgroundPosition: "0 400px" },
        },
      },
    },
  },
  safelist: [
    "neon-glow-pink",
    "neon-glow-cyan",
    "chrome-gradient",
    "glow-border",
    "vhs",
    "grid-horizon",
  ],
  plugins: [],
}
