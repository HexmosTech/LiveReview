import { combineReducers } from 'redux';
import Connector from './Connector/reducer';
import Settings from './Settings/reducer';
import Auth from './Auth/reducer';
import Organizations from './Organizations';

export default combineReducers({
    Connector,
    Settings,
    Auth,
    Organizations,
});
