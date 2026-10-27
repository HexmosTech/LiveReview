import React from 'react';
import classNames from 'classnames';
import { avatarColorFor, avatarInitialsFor } from '../../utils/avatarIdentity';

export type AvatarProps = {
    identity: { id?: number | string; name?: string; email?: string };
    size?: number;
    className?: string;
};

export const Avatar: React.FC<AvatarProps> = ({ identity, size = 36, className }) => {
    const backgroundColor = avatarColorFor(identity);
    const initials = avatarInitialsFor(identity);

    return (
        <div
            className={classNames('flex items-center justify-center rounded-full font-semibold text-white select-none shrink-0', className)}
            style={{
                width: size,
                height: size,
                backgroundColor,
                fontSize: Math.max(10, Math.round(size * 0.38)),
            }}
            aria-hidden="true"
        >
            {initials}
        </div>
    );
};
