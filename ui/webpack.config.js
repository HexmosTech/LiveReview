const path = require('path');
const HtmlWebpackPlugin = require("html-webpack-plugin");
const MiniCssExtractPlugin = require("mini-css-extract-plugin");
const TerserPlugin = require('terser-webpack-plugin');
const CssMinimizerPlugin = require('css-minimizer-webpack-plugin');
const CopyPlugin = require('copy-webpack-plugin');
const ForkTsCheckerWebpackPlugin = require('fork-ts-checker-webpack-plugin');
const BundleAnalyzerPlugin = require('webpack-bundle-analyzer').BundleAnalyzerPlugin;
const WebpackObfuscator = require('webpack-obfuscator');
const Beasties = require('beasties-webpack-plugin');
const webpack = require('webpack');
const fs = require('fs');
const metaConfig = require('./meta.config.js');

module.exports =  (env, options)=> {

    const devMode = options.mode === 'development' ? true : false;

    process.env.NODE_ENV = options.mode;

    // Explicit build mode control to prevent mistakes:
    // - LIVEREVIEW_BUILD_MODE=local     -> Use .env (local testing, user-controlled is_cloud)
    // - LIVEREVIEW_BUILD_MODE=prod      -> Use .env.prod (production deploy, is_cloud=true)
    // - LIVEREVIEW_BUILD_MODE=staging   -> Use .env.staging (staging deploy, mock AI)
    // - LIVEREVIEW_BUILD_MODE=selfhosted -> Use .env.selfhosted (Docker build, is_cloud=false)
    // - No mode set                     -> Use .env (default for local development)
    const dotenv = require('dotenv');
    const buildMode = process.env.LIVEREVIEW_BUILD_MODE || 'local';
    
    let envPath;
    let envName;
    
    if (buildMode === 'selfhosted') {
        envPath = path.resolve(__dirname, '..', '.env.selfhosted');
        envName = '.env.selfhosted';
    } else if (buildMode === 'prod') {
        envPath = path.resolve(__dirname, '..', '.env.prod');
        envName = '.env.prod';
    } else if (buildMode === 'staging') {
        envPath = path.resolve(__dirname, '..', '.env.staging');
        envName = '.env.staging';
    } else {
        envPath = path.resolve(__dirname, '..', '.env');
        envName = '.env';
    }
    
    if (fs.existsSync(envPath)) {
        dotenv.config({ path: envPath });
        console.log(`✅ Build Mode: ${buildMode.toUpperCase()} | Config: ${envName}`);
        console.log(`   LIVEREVIEW_IS_CLOUD: ${process.env.LIVEREVIEW_IS_CLOUD}`);
    } else {
        console.error(`❌ ERROR: ${envName} not found at ${envPath}`);
        console.error(`   Build mode: ${buildMode}`);
        throw new Error(`Required config file ${envName} not found`);
    }

    return {
        mode: options.mode,
        entry: path.resolve(__dirname, './src/index.tsx'),
        output: {
            path: path.resolve(__dirname, './dist'),
            filename: '[name].[contenthash].js',
            chunkFilename: '[name].[contenthash].js',
            clean: true
        },
        devtool: devMode ? 'source-map' : false,
        devServer: {
            port: 8081,
            hot: true,
            historyApiFallback: true,
            proxy: [
                {
                    context: ['/api'],
                    target: 'http://localhost:8888',
                    changeOrigin: true,
                }
            ]
        },
        resolve: {
            extensions: ['.js', '.jsx', '.json', '.ts', '.tsx'],
            alias: {
                '@components': path.resolve(__dirname, 'src/components/'),
                '@constants': path.resolve(__dirname, 'src/constants/'),
                '@hooks': path.resolve(__dirname, 'src/hooks/'),
                '@services': path.resolve(__dirname, 'src/services/'),
                '@store': path.resolve(__dirname, 'src/store/'),
                '@styles': path.resolve(__dirname, 'src/styles/'),
                '@utils': path.resolve(__dirname, 'src/utils/'),
            }
        },
        module: {
            rules: [
                {
                    test: /\.(ts|tsx)$/,
                    loader: 'babel-loader'
                },
                {
                    test: /\.css$/i,
                    // include: path.resolve(__dirname, 'src'),
                    use: [
                        devMode ? 'style-loader' : MiniCssExtractPlugin.loader,
                        {
                            loader: "css-loader", 
                            options: {
                                sourceMap: true
                            }
                        }, 
                        {
                            loader: 'postcss-loader'
                        }
                    ],
                },
                // { 
                //     test: /\.(woff|woff2|ttf|eot)$/,  
                //     loader: "file-loader",
                //     options: {
                //         name: '[name].[contenthash].[ext]',
                //     }
                // },
                {
                    test: /\.(woff|woff2|ttf|eot)$/,
                    type: 'asset/resource',
                },
                // { 
                //     test: /\.(png|jpg|gif|svg)$/,  
                //     loader: "file-loader",
                //     options: {
                //         name: '[name].[contenthash].[ext]',
                //     }
                // },
                {
                    test: /\.(png|jpe?g|gif|svg)$/i,
                    type: 'asset/resource'
                },
            ]
        },
        plugins: [
            // need to use ForkTsCheckerWebpackPlugin because Babel loader ignores the compilation errors for Typescript
            new ForkTsCheckerWebpackPlugin(),
            // Define plugin to inject environment variables
            new webpack.DefinePlugin({
                // Support unified API_URL with fallback to framework-specific variable
                'process.env.API_URL': JSON.stringify(process.env.API_URL || process.env.REACT_APP_API_URL),
                'process.env.REACT_APP_API_URL': JSON.stringify(process.env.API_URL || process.env.REACT_APP_API_URL),
                // Expose cloud/self-hosted flag from root .env to browser
                'process.env.LIVEREVIEW_IS_CLOUD': JSON.stringify(process.env.LIVEREVIEW_IS_CLOUD || ''),
                // Cloud-only: Analytics and notification secrets (empty = disabled)
                'process.env.LR_CLARITY_ID': JSON.stringify(process.env.LR_CLARITY_ID || ''),
                'process.env.LR_DISCORD_PROXY_URL': JSON.stringify(process.env.LR_DISCORD_PROXY_URL || ''),
                'process.env.LR_DISCORD_WEBHOOK_URL': JSON.stringify(process.env.LR_DISCORD_WEBHOOK_URL || ''),
                'process.env.LR_LISTMONK_URL': JSON.stringify(process.env.LR_LISTMONK_URL || ''),
                'process.env.LR_LISTMONK_LIST_ID': JSON.stringify(process.env.LR_LISTMONK_LIST_ID || ''),
            }),
            // webpack 5 no longer auto-polyfills Node globals. Some deps (e.g. react-draggable,
            // used by react-grid-layout for the customizable dashboard) reference a bare `process`
            // at runtime (process.env.DRAGGABLE_DEBUG) outside of what DefinePlugin above replaces,
            // which throws "process is not defined" in the browser without this.
            new webpack.ProvidePlugin({
                process: require.resolve('process/browser'),
            }),
            new MiniCssExtractPlugin({
                // Options similar to the same options in webpackOptions.output
                // both options are optional
                filename: devMode ? '[name].css' : '[name].[contenthash].css',
                chunkFilename: devMode ? '[name].css' : '[name].[contenthash].css',
            }),
            // copy static files from public folder to build directory
            new CopyPlugin({
                patterns: [
                    { 
                        from: "public/**/*", 
                        globOptions: {
                            ignore: ["**/index.html"],
                        },
                    },
                    // Copy assets folder to root of build
                    {
                        from: "assets",
                        to: "assets",
                        force: true
                    }
                ],
            }),
            new HtmlWebpackPlugin({
                template: './public/index.html',
                filename: 'index.html',
                title: metaConfig.title,
                favicon: path.resolve(__dirname, './assets/favicon.svg'),
                meta: metaConfig.meta,
                minify: {
                    html5                          : true,
                    collapseWhitespace             : true,
                    minifyCSS                      : true,
                    minifyJS                       : true,
                    minifyURLs                     : false,
                    removeComments                 : true,
                    removeEmptyAttributes          : true,
                    removeOptionalTags             : true,
                    removeRedundantAttributes      : true,
                    removeScriptTypeAttributes     : true,
                    removeStyleLinkTypeAttributese : true,
                    useShortDoctype                : true
                }
            }),
            // Inline critical CSS and load the rest asynchronously to avoid render-blocking
            !devMode ? new Beasties({
                preload: 'swap',
                noscriptFallback: true,
                compress: true
            }) : false,
            // !devMode ? new CleanWebpackPlugin() : false,
            !devMode && process.env.ANALYZE_BUNDLE && !process.env.CI ? new BundleAnalyzerPlugin() : false,
            // Optional JavaScript obfuscation for production builds (enable with OBFUSCATE=true)
            // NOTE: WebpackObfuscator does not read an `exclude` key from its options object
            // (that's not part of its API - see node_modules/webpack-obfuscator/README.md).
            // File exclusion must go through the second constructor argument (a multimatch glob
            // list matched against the *compiled* bundle name). Obfuscating third-party vendor
            // code adds real parse/execute overhead for zero benefit (nothing proprietary in it),
            // so vendor chunks are excluded here; only first-party app code gets obfuscated.
            !devMode && process.env.OBFUSCATE ? new WebpackObfuscator({
                compact: true,
                controlFlowFlattening: false,
                deadCodeInjection: false,
                debugProtection: false,
                disableConsoleOutput: true,
                identifierNamesGenerator: 'mangled',
                log: false,
                renameGlobals: false,
                rotateStringArray: true,
                selfDefending: false,
                shuffleStringArray: true,
                splitStrings: false,
                stringArray: true,
                stringArrayThreshold: 0.75,
                transformObjectKeys: false,
                unicodeEscapeSequence: false,
            }, ['runtime.*.js', 'vendor-*.js']) : false
        ].filter(Boolean),
        optimization: {
            splitChunks: {
                chunks: 'all',
                // Cap chunk size so a single cache group can't balloon back into one huge
                // file as dependencies are added (helps HTTP/2 parallel fetch + browser caching).
                maxSize: 300000,
                cacheGroups: {
                    // Core framework deps: needed on every route including login, so these load
                    // eagerly - keep this group small and stable so it caches well across deploys.
                    framework: {
                        test: /[\\/]node_modules[\\/](react|react-dom|react-router|react-router-dom|redux|react-redux|@reduxjs)[\\/]/,
                        name: 'vendor-framework',
                        chunks: 'all',
                        priority: 20
                    },
                    // Charting/visualization libs: only ever imported from lazy-loaded routes
                    // (Dashboard widgets, Reports, Chatbot). chunks: 'async' keeps them out of
                    // the eagerly-loaded initial bundle - they're only fetched when that route
                    // actually mounts, instead of blocking every page load (including login).
                    charts: {
                        test: /[\\/]node_modules[\\/](echarts|echarts-for-react|recharts|vega|vega-lite|vega-embed|react-vega|d3-[^\\/]*)[\\/]/,
                        name: 'vendor-charts',
                        chunks: 'async',
                        priority: 15
                    },
                    // Grid layout / animation / pdf export / table / timezone-data libs: same
                    // story, only used from lazy routes (Dashboard, Reports, Explore, Reviews,
                    // UserManagement, Settings, Licenses). moment-timezone in particular ships a
                    // ~700KB packed locale-data file that has no business loading before login.
                    heavy: {
                        test: /[\\/]node_modules[\\/](react-grid-layout|framer-motion|jspdf|jspdf-autotable|@tanstack[\\/]react-table|moment-timezone)[\\/]/,
                        name: 'vendor-heavy',
                        chunks: 'async',
                        priority: 15
                    },
                    // Everything else third-party. Deliberately no static `name` string here:
                    // forcing every remaining node_modules module into one named chunk would
                    // merge sync-needed deps (e.g. used eagerly from the app shell) with deps
                    // only ever reached from lazy routes into the same physical file - and since
                    // a chunk is an atomic download, that drags the lazy-only code into the eager
                    // initial payload too. The name *function* below still splits purely by
                    // webpack's actual usage-graph (so lazy-only vendor code stays out of the
                    // initial load) while keeping every resulting file prefixed `vendor-` so the
                    // WebpackObfuscator excludes glob above still matches all of them.
                    vendor: {
                        test: /[\\/]node_modules[\\/]/,
                        chunks: 'all',
                        priority: -10,
                        name(module, chunks) {
                            return `vendor-misc-${chunks.map((c) => c.name || c.id).join('~')}`;
                        }
                    }
                }
            },
            runtimeChunk: 'single',
            minimizer: [
                new TerserPlugin({
                    extractComments: false, // Don't extract comments to separate file
                    terserOptions: {
                        compress: {
                            drop_console: true,
                            drop_debugger: true,
                            pure_funcs: ['console.log', 'console.info', 'console.debug', 'console.warn'],
                            passes: 2, // Multiple compression passes
                            dead_code: true,
                            drop_debugger: true,
                            conditionals: true,
                            evaluate: true,
                            booleans: true,
                            loops: true,
                            unused: true,
                            hoist_funs: true,
                            keep_fargs: false,
                            hoist_vars: true,
                            if_return: true,
                            join_vars: true,
                            side_effects: true // Keep side effects to avoid breaking libraries like moment.js
                        },
                        mangle: {
                            toplevel: false, // Don't mangle top-level names to avoid breaking libraries
                            eval: true,
                            keep_fnames: false,
                            properties: false, // Disable property mangling to prevent breaking moment.js and other libraries
                        },
                        format: {
                            comments: false,
                            beautify: false
                        }
                    }
                }), 
                new CssMinimizerPlugin()
            ]
        },
    }

};