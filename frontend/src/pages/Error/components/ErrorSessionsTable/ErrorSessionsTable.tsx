import classNames from 'classnames';
import moment from 'moment';
import React, { useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { PlayerSearchParameters } from '../../../Player/PlayerHook/utils';
import styles from '../../ErrorPage.module.scss';
import commonStyles from '../../../../Common.module.scss';
import { ErrorGroup, Maybe } from '../../../../graph/generated/schemas';

interface Props {
    errorGroup: Maybe<ErrorGroup> | undefined;
}

const ErrorSessionsTable = ({ errorGroup }: Props) => {
    const { organization_id } = useParams<{
        organization_id: string;
    }>();
    const [errorActivityCount, setErrorActivityCount] = useState(20);

    return (
        <div className={styles.fieldWrapper} style={{ paddingBottom: '40px' }}>
            <div className={styles.section}>
                <div className={styles.collapsible}>
                    <div
                        className={classNames(
                            styles.triggerWrapper,
                            styles.errorLogDivider
                        )}
                    >
                        <div className={styles.errorLogsTitle}>
                            <span>Error ID</span>
                            <span>Session ID</span>
                            <span>Visited URL</span>
                            <span>Browser</span>
                            <span>OS</span>
                            <span>Timestamp</span>
                        </div>
                    </div>
                    <div className={styles.errorLogWrapper}>
                        {errorGroup?.metadata_log
                            .slice(
                                Math.max(
                                    errorGroup?.metadata_log.length -
                                        errorActivityCount,
                                    0
                                )
                            )
                            .reverse()
                            .map((e, i) => (
                                <Link
                                    to={`/${organization_id}/sessions/${
                                        e?.session_id
                                    }${
                                        e?.timestamp
                                            ? `?${PlayerSearchParameters.errorId}=${e.error_id}`
                                            : ''
                                    }`}
                                    key={i}
                                >
                                    <div
                                        key={i}
                                        className={styles.errorLogItem}
                                    >
                                        <span>{e?.error_id}</span>
                                        <span>{e?.session_id}</span>
                                        <div className={styles.errorLogCell}>
                                            <span
                                                className={
                                                    styles.errorLogOverflow
                                                }
                                            >
                                                {e?.visited_url}
                                            </span>
                                        </div>
                                        <span>{e?.browser}</span>
                                        <span>{e?.os}</span>
                                        <span>
                                            {moment(e?.timestamp).format(
                                                'D MMMM YYYY, HH:mm:ss'
                                            )}
                                        </span>
                                    </div>
                                </Link>
                            ))}
                        {errorGroup?.metadata_log.length &&
                            errorActivityCount <
                                errorGroup?.metadata_log.length && (
                                <button
                                    onClick={() =>
                                        setErrorActivityCount(
                                            errorActivityCount + 20
                                        )
                                    }
                                    className={classNames(
                                        commonStyles.secondaryButton,
                                        styles.errorLogButton
                                    )}
                                >
                                    Show more...
                                </button>
                            )}
                    </div>
                </div>
            </div>
        </div>
    );
};

export default ErrorSessionsTable;
