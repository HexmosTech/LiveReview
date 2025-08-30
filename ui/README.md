# React-Redux Boilerplate

React-Redux Boilerplate is a project template that provides an easier and faster way to start a React+Redux project with TypeScript. It comes with various features, including React, Redux, TypeScript, Webpack, Jest, Tailwind, EsLint, and Prettier.

## Features
### React
A popular JavaScript library for building user interfaces. It allows developers to create reusable UI components and provides a declarative approach to rendering views.

### Redux
A predictable state container for JavaScript apps. It provides a centralized store for managing application state and makes it easy to reason about state changes.

### Redux-Toolkit
An opinionated, batteries-included package for efficient Redux development. It includes utilities for simplified Redux store setup, creating reducers and actions, and handling immutable updates.

### TypeScript
A superset of JavaScript that adds optional static type checking and other features to the language. It helps catch errors earlier in the development process and improves code readability and maintainability.

### Webpack
A module bundler that allows developers to bundle and optimize their code for production. It supports a wide range of asset types, including JavaScript, CSS, and images, and provides powerful features for code splitting and lazy loading.

### Jest
A popular testing framework for JavaScript applications. It provides a simple and intuitive API for writing tests and supports a wide range of features, including mocking, code coverage, and snapshot testing.

### Tailwind
A utility-first CSS framework that provides a set of pre-defined styles for common UI elements. It allows developers to quickly build responsive and consistent user interfaces without writing custom CSS.

### EsLint
A popular linter for JavaScript that helps identify and fix syntax errors, coding style issues, and other common programming errors. It provides a wide range of customizable rules and integrates with many popular text editors and build tools.

### Prettier
An opinionated code formatter that helps ensure code consistency and readability across teams. It supports a wide range of languages and integrates with many popular text editors and build tools.

## Getting Started
Before you start, make sure you have a fresh version of [Node.js](https://nodejs.org/en/) and NPM installed. The current Long Term Support (LTS) release is an ideal starting point.

1. Fork this repository and clone your own fork to your computer: 
    ```sh
    git clone https://github.com/YOUR_USERNAME/react-redux-boilerplate.git
    ```


2. From the project's root directory, install the required packages (dependencies):

    ```sh
    npm install
    ```

3. To run and test the app on your local machine (http://localhost:8080):

    ```sh
    npm run start
    ```

    This will start a server instance and begin listening for connections from localhost on port `8080`.

4. To build/deploye the app, you can run:

    ```sh
    npm run build
    ```

    This will place all files needed for deployment into the `/dist` directory.

## Project Structure

```sh
├── public  
    ├── favicon.ico
    ├── index.html              # html template for the app
    ├── thumbnail.jpg           # an image will be used in og:image meta tag
├── src                         # Source code.
    ├── components              # React components
    ├── constants               # app-wide constants (text, URLs, themes and etc)
    ├── contexts                # React contexts
    ├── hooks                   # reusable custom hooks
    ├── pages                   # Page components
    ├── services                # API calls
    ├── static                  # static assets
    ├── store                   # Redux store
        ├── ToDo                # contains reducer, selectors and thunks for ToDo "slice" of the redux store
        ├── configureStore.ts   # configure the app's redux store
        ├── getPreloadedState.ts 
        ├── rootReducers.ts     # combine reducers from all slices
    ├── styles                  # app-wide styles
    ├── types                   # type definitions
    ├── utils                   # utility functions
    └── index.tsx               # entry point for the app
├── .babelrc                    # Babel configuration
├── .eslintrc.js                # ESLint configuration
├── .prettierrc.js              # Prettier configuration
├── tsconfig.json               # TypeScript configuration
├── webpack.config.js           # Webpack configurations
├── tailwind.config.js          # Tailwind CSS configurations
├── postcss.config.js           # PostCSS configurations
├── meta.config.js              # meta tags configuration
```

## Issues
Find a bug or want to request a new feature? Please let me know by submitting an issue.

## Licensing
Copyright 2022 Jinnan Zhang

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.