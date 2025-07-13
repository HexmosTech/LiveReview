type UrlHashParamKey = 'mapCenter';

const hashParams = new URLSearchParams(window.location.hash.slice(1));

/**
 * update Hash Params in the URL using data from hashParams
 */
export const updateHashParams = (key: UrlHashParamKey, value: string) => {
    if (value === undefined || value === null) {
        hashParams.delete(key);
    } else {
        hashParams.set(key, value);
    }

    window.location.hash = hashParams.toString();
};

export const getHashParamValueByKey = (key: UrlHashParamKey): string => {
    if (!hashParams.has(key)) {
        return null;
    }

    return hashParams.get(key);
};
