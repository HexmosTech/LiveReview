import { combineReducers } from 'redux';
import Connector from './Connector/reducer';
import Settings from './Settings/reducer';

export default combineReducers({
    Connector,
    Settings,
});
