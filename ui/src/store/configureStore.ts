import { configureStore } from '@reduxjs/toolkit';

import rootReducer from './rootReducer';

import getPreloadedState from './getPreloadedState';
import { useSelector, useDispatch } from 'react-redux';

export type RootState = ReturnType<typeof rootReducer>;

export type PartialRootState = Partial<RootState>;

const configureAppStore = (preloadedState: PartialRootState = {}) => {
    const store = configureStore({
        reducer: rootReducer,
        preloadedState,
    });

    return store;
};

export type AppStore = ReturnType<typeof configureAppStore>;

export type StoreDispatch = ReturnType<typeof configureAppStore>['dispatch'];

export type StoreGetState = ReturnType<typeof configureAppStore>['getState'];

// Use throughout app instead of plain `useDispatch` and `useSelector`
// @see https://redux-toolkit.js.org/tutorials/typescript#define-typed-hooks
export const useAppDispatch = useDispatch.withTypes<StoreDispatch>();
export const useAppSelector = useSelector.withTypes<RootState>();

export { getPreloadedState };

export default configureAppStore;
