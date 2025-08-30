import { PartialRootState } from './configureStore';

const getPreloadedState = (): PartialRootState => {
    return {
        Connector: {
            connectors: [],
            isLoading: false,
            error: null,
        },
    };
};

export default getPreloadedState;
