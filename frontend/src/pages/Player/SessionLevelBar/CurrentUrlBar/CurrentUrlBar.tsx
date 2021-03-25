import React, { useState } from 'react';
import styles from './CurrentUrlBar.module.scss';
import { ReactComponent as SearchIcon } from '../../../../static/search.svg';
import { searchIcon } from '../../../Sessions/SessionsPage.module.scss';
import { FaCopy } from 'react-icons/fa';
import { message } from 'antd';

export const CurrentUrlBar = ({ url }: { url: string }) => {
    const [hover, setHover] = useState(false);
    //     <SessionToken
    //     icon={<URLIcon />}
    //     tooltipTitle="The current URL the user is on."
    // >
    //     <a href={currentUrl} target="_blank" rel="noreferrer">
    //         {currentUrl}
    //     </a>
    // </SessionToken>
    return (
        <div
            className={styles.urlBarWrapper}
            onMouseEnter={() => setHover(true)}
            onMouseLeave={() => setHover(false)}
        >
            <SearchIcon className={styles.searchIcon} />
            <a
                className={styles.urlLink}
                target="_blank"
                href={url}
                rel="noreferrer"
            >
                {url}
            </a>
            {hover && (
                <FaCopy
                    className={styles.copyIcon}
                    onClick={() => {
                        navigator.clipboard.writeText(url);
                        message.success('Copied url to clipboard');
                    }}
                />
            )}
        </div>
    );
};
