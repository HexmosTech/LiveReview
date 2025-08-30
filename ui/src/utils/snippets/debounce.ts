type CallbackFunction = (...args: any[]) => any;

export const debounce = (func: CallbackFunction, delay = 200) => {
    let timer: NodeJS.Timeout;

    return function (...args: any[]) {
        clearTimeout(timer);

        timer = setTimeout(() => {
            func.apply(this, args);
        }, delay);
    };
};
