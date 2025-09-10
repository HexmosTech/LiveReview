import { configureStore, Middleware } from '@reduxjs/toolkit';

import rootReducer from './rootReducer';

import getPreloadedState from './getPreloadedState';
import { useSelector, useDispatch } from 'react-redux';

export type RootState = ReturnType<typeof rootReducer>;

export type PartialRootState = Partial<RootState>;

const configureAppStore = (preloadedState: PartialRootState = {}) => {
    // Simple thunk middleware to avoid importing redux-thunk (prevents ENOENT in some envs)
    const simpleThunk: Middleware = ({ dispatch, getState }) => (next) => (action) => {
        if (typeof action === 'function') {
            return (action as any)(dispatch, getState, undefined);
        }
        return next(action);
    };

    const store = configureStore({
        reducer: rootReducer,
        preloadedState,
        middleware: (getDefault) => getDefault({ thunk: false }).concat(simpleThunk),
    });

    return store;
};

export type AppStore = ReturnType<typeof configureAppStore>;

// Dispatch type widened to accept both plain actions and thunk functions.
export type StoreDispatch = (action: any) => any;

export type StoreGetState = ReturnType<typeof configureAppStore>['getState'];

// Use throughout app instead of plain `useDispatch` and `useSelector`
// @see https://redux-toolkit.js.org/tutorials/typescript#define-typed-hooks
export const useAppDispatch = useDispatch.withTypes<StoreDispatch>();
export const useAppSelector = useSelector.withTypes<RootState>();

export { getPreloadedState };

export default configureAppStore;
