import { createSelector } from '@reduxjs/toolkit';
import { RootState } from '../configureStore';
import { Connector } from './reducer';

export const selectAllConnectors = createSelector(
    (state: RootState) => state.Connector.connectors,
    (connectors) => connectors
);

export const selectConnectorById = (id: string) => 
    createSelector(
        (state: RootState) => state.Connector.connectors,
        (connectors) => connectors.find(connector => connector.id === id)
    );

export const selectIsLoading = createSelector(
    (state: RootState) => state.Connector.isLoading,
    (isLoading) => isLoading
);

export const selectError = createSelector(
    (state: RootState) => state.Connector.error,
    (error) => error
);
