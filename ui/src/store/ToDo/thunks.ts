import { RootState, StoreDispatch, StoreGetState } from '../configureStore';
import { taskToggled } from './reducer';

// Good resource about what "thunks" are, and why they're used for writing Redux logic: https://redux.js.org/usage/writing-logic-thunks
export const toggleTaskStatus =
    (id: string) =>
    async (dispatch: StoreDispatch, getState: StoreGetState) => {
        try {
            // do some async work (e.g. check if the new webmap id is an valid ArcGIS Online Item)
            // ...

            // now everything is ready and we can dispatch the new webmap Id to the reducer to trigger the state change
            dispatch(taskToggled(id));
        } catch (err) {
            console.error(err);
        }
    };
