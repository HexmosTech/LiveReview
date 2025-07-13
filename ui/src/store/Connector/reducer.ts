import { createSlice, PayloadAction } from '@reduxjs/toolkit';

export type ConnectorType = 'gitlab' | 'github' | 'custom';

export type Connector = {
    id: string;
    name: string;
    type: ConnectorType;
    url: string;
    apiKey: string;
    createdAt: string;
};

type ConnectorState = {
    connectors: Connector[];
    isLoading: boolean;
    error: string | null;
};

const initialState: ConnectorState = {
    connectors: [],
    isLoading: false,
    error: null,
};

const connectorSlice = createSlice({
    name: 'connector',
    initialState,
    reducers: {
        addConnector: (
            state,
            action: PayloadAction<Omit<Connector, 'id' | 'createdAt'>>
        ) => {
            const newConnector: Connector = {
                ...action.payload,
                id: Date.now().toString(),
                createdAt: new Date().toISOString(),
            };
            state.connectors.push(newConnector);
        },
        removeConnector: (state, action: PayloadAction<string>) => {
            state.connectors = state.connectors.filter(
                (connector) => connector.id !== action.payload
            );
        },
        updateConnector: (
            state,
            action: PayloadAction<Partial<Connector> & { id: string }>
        ) => {
            const index = state.connectors.findIndex(
                (c) => c.id === action.payload.id
            );
            if (index !== -1) {
                state.connectors[index] = {
                    ...state.connectors[index],
                    ...action.payload,
                };
            }
        },
    },
});

export const { addConnector, removeConnector, updateConnector } = connectorSlice.actions;

export default connectorSlice.reducer;
