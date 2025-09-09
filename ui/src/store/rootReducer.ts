import { combineReducers } from 'redux';
import Connector from './Connector/reducer';
import Settings from './Settings/reducer';
import Auth from './Auth/reducer';
import Organizations from './Organizations';
import License from './License/slice';

export default combineReducers({
    Connector,
    Settings,
    Auth,
    Organizations,
    License,
});
