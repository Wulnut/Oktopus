module.exports = {
  testEnvironment: 'jsdom',
  moduleNameMapper: {
    '^src/(.*)$': '<rootDir>/src/$1',
  },
  transform: {
    '^.+\\.(js|jsx)$': ['babel-jest', {
      presets: ['next/babel'],
    }],
  },
  transformIgnorePatterns: [
    '/node_modules/(?!(@mui|@emotion|@heroicons)/)',
  ],
  setupFilesAfterSetup: [],
};
