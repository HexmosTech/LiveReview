import React, { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { Button, Input, SingleSelectField } from '../UIPrimitives';
import { useAppSelector } from '../../store/configureStore';
import apiClient from '../../api/apiClient';

const FOUNDER_LINKEDIN_URL = 'https://www.linkedin.com/in/shrijith-venkatramana-32741b2b0/';

const DEV_COUNT_OPTIONS = ['1-10', '11-50', '51-200', '201-500', '501-1000', '1000+'].map(v => ({ value: v, label: v }));

// ISO 3166 codes; names come from the browser's Intl so we don't ship a country list.
const COUNTRY_CODES = 'AF AL DZ AD AO AG AR AM AU AT AZ BS BH BD BB BY BE BZ BJ BT BO BA BW BR BN BG BF BI KH CM CA CV CF TD CL CN CO KM CG CD CR CI HR CU CY CZ DK DJ DM DO EC EG SV GQ ER EE SZ ET FJ FI FR GA GM GE DE GH GR GD GT GN GW GY HT HN HK HU IS IN ID IR IQ IE IL IT JM JP JO KZ KE KI KW KG LA LV LB LS LR LY LI LT LU MO MG MW MY MV ML MT MH MR MU MX FM MD MC MN ME MA MZ MM NA NR NP NL NZ NI NE NG KP MK NO OM PK PW PS PA PG PY PE PH PL PT PR QA RO RU RW KN LC VC WS SM ST SA SN RS SC SL SG SK SI SB SO ZA KR SS ES LK SD SR SE CH SY TW TJ TZ TH TL TG TO TT TN TR TM TV UG UA AE GB US UY UZ VU VA VE VN YE ZM ZW'.split(' ');
const regionNames = new Intl.DisplayNames(['en'], { type: 'region' });
const COUNTRY_OPTIONS = COUNTRY_CODES.map(code => ({ value: code, label: regionNames.of(code) || code }))
    .sort((a, b) => a.label.localeCompare(b.label));

const EMPTY_FORM = { name: '', company: '', email: '', jobTitle: '', developers: '', country: '', message: '' };

const Required = () => <span className="text-red-400">*</span>;
const FieldLabel: React.FC<{ children: React.ReactNode }> = ({ children }) => (
    <span className="block text-sm font-medium text-slate-300 mb-1">{children}</span>
);

const BENEFITS = [
    {
        text: 'Keep your code, infrastructure, and AI keys entirely under your control',
        tone: 'bg-blue-500/20 text-blue-400',
        path: 'M12 3l7 3v5c0 4.5-3 8.3-7 9.5C8 19.3 5 15.5 5 11V6l7-3z',
    },
    {
        text: 'Use the tools and models that fit your team',
        tone: 'bg-violet-500/20 text-violet-400',
        path: 'M8 9l-3 3 3 3m8-6l3 3-3 3M13.5 7l-3 10',
    },
    {
        text: 'Reduce spend without sacrificing capability',
        tone: 'bg-emerald-500/20 text-emerald-400',
        path: 'M12 4v16m4-12.5c-.8-1-2.2-1.5-4-1.5-2.2 0-4 1.1-4 3s1.8 2.6 4 3 4 1.1 4 3-1.8 3-4 3c-1.8 0-3.2-.6-4-1.5',
    },
];

interface EnterpriseModalProps {
    show: boolean;
    onClose: () => void;
}

export const EnterpriseModal: React.FC<EnterpriseModalProps> = ({ show, onClose }) => {
    const [form, setForm] = useState(EMPTY_FORM);
    const [submitted, setSubmitted] = useState(false);
    const [devCountError, setDevCountError] = useState(false);
    const [sending, setSending] = useState(false);
    const [sendError, setSendError] = useState<string | null>(null);
    const user = useAppSelector(state => state.Auth.user);

    // Prefill name and email from the logged-in user, keeping anything already typed.
    useEffect(() => {
        if (!show || !user) return;
        setForm(f => ({ ...f, name: f.name || user.name || '', email: f.email || user.email || '' }));
    }, [show, user]);

    useEffect(() => {
        if (!show) return;
        const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
        document.addEventListener('keydown', onKey);
        return () => document.removeEventListener('keydown', onKey);
    }, [show, onClose]);

    if (!show) return null;

    const set = (key: keyof typeof EMPTY_FORM) =>
        (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
            setForm(f => ({ ...f, [key]: e.target.value }));

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!form.developers) {
            setDevCountError(true);
            return;
        }
        setSendError(null);
        setSending(true);
        try {
            // The backend formats the Discord message, so the webhook never reaches the browser.
            await apiClient.post('/api/v1/enterprise-enquiry', {
                name: form.name,
                company: form.company,
                email: form.email,
                job_title: form.jobTitle,
                developers: form.developers,
                country: form.country ? regionNames.of(form.country) || form.country : '',
                message: form.message,
            });
            setSubmitted(true);
            setForm(EMPTY_FORM);
        } catch (err) {
            console.error('[LiveReview] Enterprise enquiry failed', err);
            setSendError('Could not send your enquiry. Please try again, or email info@hexmos.com.');
        } finally {
            setSending(false);
        }
    };

    const handleClose = () => {
        if (sending) return;
        setSubmitted(false);
        setSendError(null);
        onClose();
    };

    return createPortal(
        <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
            <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={handleClose} />
            <div
                role="dialog"
                aria-modal="true"
                aria-labelledby="enterprise-modal-title"
                className="relative bg-slate-800 rounded-xl border border-slate-600 shadow-2xl max-w-xl w-full max-h-[90vh] overflow-y-auto"
            >
                <button
                    type="button"
                    onClick={handleClose}
                    aria-label="Close"
                    className="absolute top-4 right-4 text-slate-400 hover:text-white"
                >
                    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                    </svg>
                </button>

                <div className="p-6 sm:p-7">
                    <span className="inline-flex items-center rounded-full bg-blue-500/20 px-2.5 py-0.5 text-xs font-semibold tracking-wide text-blue-300">
                        ENTERPRISE
                    </span>
                    <h2 id="enterprise-modal-title" className="mt-3 text-2xl font-bold text-white leading-snug pr-8">
                        For most engineering teams, we recommend going self-hosted
                    </h2>

                    <ul className="mt-6 space-y-3">
                        {BENEFITS.map(b => (
                            <li key={b.text} className="flex items-center gap-3 text-sm text-slate-300">
                                <span className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg ${b.tone}`}>
                                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={b.path} />
                                    </svg>
                                </span>
                                {b.text}
                            </li>
                        ))}
                    </ul>

                    <div className="my-6 h-px bg-slate-700" />

                    {submitted ? (
                        <div className="py-6 text-center">
                            <p className="text-lg font-semibold text-white">Thanks, we'll be in touch soon.</p>
                            <p className="mt-1 text-sm text-slate-400">Our team usually replies within one business day.</p>
                            <Button variant="outline" className="mt-5" onClick={handleClose}>Close</Button>
                        </div>
                    ) : (
                        <form onSubmit={handleSubmit} className="space-y-4">
                            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                                <Input label={<>Name <Required /></>} required placeholder="Enter your name" value={form.name} onChange={set('name')} />
                                <Input label={<>Company name <Required /></>} required placeholder="Enter your company name" value={form.company} onChange={set('company')} />
                                <Input label={<>Work email <Required /></>} required type="email" placeholder="Enter your work email" value={form.email} onChange={set('email')} />
                                <Input label={<>Job title <Required /></>} required placeholder="Enter your job title" value={form.jobTitle} onChange={set('jobTitle')} />
                                <div>
                                    <FieldLabel>Number of developers <Required /></FieldLabel>
                                    <SingleSelectField
                                        label="Number of developers"
                                        placeholder="Select a number of developers"
                                        allowClear={false}
                                        options={DEV_COUNT_OPTIONS}
                                        value={form.developers}
                                        onChange={v => { setForm(f => ({ ...f, developers: v })); setDevCountError(false); }}
                                    />
                                    {devCountError && <p className="mt-1 text-sm text-red-400">Please select a number of developers</p>}
                                </div>
                                <div>
                                    <FieldLabel>Country</FieldLabel>
                                    <SingleSelectField
                                        label="Country"
                                        placeholder="Select your country"
                                        searchable
                                        allowClear={false}
                                        options={COUNTRY_OPTIONS}
                                        value={form.country}
                                        onChange={v => setForm(f => ({ ...f, country: v }))}
                                    />
                                </div>
                            </div>
                            <div>
                                <label htmlFor="enterprise-message" className="block text-sm font-medium text-slate-300 mb-1">
                                    How can we help?
                                </label>
                                <textarea
                                    id="enterprise-message"
                                    rows={3}
                                    placeholder="Enter your message here"
                                    value={form.message}
                                    onChange={set('message')}
                                    className="w-full rounded-lg border border-slate-600 bg-slate-700 px-4 py-2.5 text-white outline-none transition-all duration-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-400"
                                />
                            </div>
                            {sendError && <p className="text-sm text-red-400">{sendError}</p>}
                            <Button type="submit" variant="primary" disabled={sending} className="w-full !py-3 text-base font-semibold">
                                {sending ? 'Sending...' : 'Talk to Our Team'}
                            </Button>
                            <p className="text-center text-sm text-slate-400">
                                Prefer a quick 1:1?{' '}
                                <a href={FOUNDER_LINKEDIN_URL} target="_blank" rel="noopener noreferrer" className="font-medium text-blue-400 underline hover:text-blue-300">
                                    DM our founder on LinkedIn
                                </a>
                            </p>
                        </form>
                    )}
                </div>
            </div>
        </div>,
        document.body
    );
};
